#!/usr/bin/env python3
"""Capture the same fixture in Chromium and the native Onix renderer (Linux).
Requires agent-browser, Xvfb, xwd, Go, and Pillow. No editor is launched.
"""
import argparse
import functools
import hashlib
import http.server
import json
import os
from pathlib import Path
import select
import struct
import subprocess
import tempfile
import threading
from PIL import Image, ImageChops, ImageStat, ImageFilter

ROOT = Path(__file__).resolve().parents[2]
RESOURCES = Path(__file__).resolve().parent / 'resources'


def run(args, **kwargs):
    return subprocess.run(args, check=True, text=True, capture_output=True, cwd=ROOT, timeout=120, **kwargs).stdout.strip()


def text_ink_error(reference, actual, rect):
    # Exclude a two-pixel box boundary so fractional background edges are not
    # counted as glyph errors. Keep the union of ink pixels from both images.
    bounds=(int(rect['X'])+2,int(rect['Y'])+2,int(rect['X']+rect['Width'])-2,int(rect['Y']+rect['Height'])-2)
    a,b=reference.crop(bounds),actual.crop(bounds)
    background=a.getpixel((a.width-1,0))
    count=0
    total=[0,0,0]
    for left,right in zip(a.getdata(),b.getdata()):
        if max(*(abs(left[c]-background[c]) for c in range(3)),*(abs(right[c]-background[c]) for c in range(3)))<=12:
            continue
        count+=1
        for c in range(3):total[c]+=abs(left[c]-right[c])
    return {'inkInteriorPixels':count,'inkInteriorMeanAbsoluteRGB':[value/max(1,count) for value in total]}


def read_ready(process, expected=None):
    if not select.select([process.stdout], [], [], 30)[0]:
        raise RuntimeError('Renderer readiness timeout')
    line = process.stdout.readline().strip()
    if expected is not None and line != expected:
        raise RuntimeError(f'Unexpected renderer output: {line!r}')
    return line


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--fixture', choices=['batch1', 'batch2', 'batch3', 'batch4', 'batch5', 'batch6', 'batch7', 'text', 'line-height', 'painting-pending', 'workbench'], default='batch1')
    parser.add_argument('--update-browser', action='store_true', help='Explicitly replace checked-in browser geometry baseline')
    args = parser.parse_args()
    global RESOURCES
    fixture = RESOURCES / (args.fixture + '.html')
    RESOURCES = RESOURCES / args.fixture
    RESOURCES.mkdir(exist_ok=True)
    session = f'onix-compat-{os.getpid()}'
    browser = ['agent-browser', '--session', session]
    handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=str(ROOT))
    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    xvfb = native = None
    try:
        run(browser + ['open', f'http://127.0.0.1:{server.server_port}/tests/htmlcompat/resources/{args.fixture}.html'])
        run(browser + ['set', 'viewport', '1200', '650'])
        script = '''(async()=>{
          await document.fonts.load('16px Compat');
          await document.fonts.ready;
          if(!document.fonts.check('16px Compat'))throw Error('Fixture font did not load');
          const boxes={}, text={}, textRects={}, textCases={}, controlDisplays={}, standaloneControlDisplays={};
          const images=%(images)s;
          for(const e of document.querySelectorAll('[id]')){
            const r=e.getBoundingClientRect();boxes[e.id]={X:r.x,Y:r.y,Width:r.width,Height:r.height};
            if(e.id.startsWith('text-')){const range=document.createRange();range.setStart(e.firstChild,0);range.setEnd(e.firstChild,1);const tr=range.getBoundingClientRect();text[e.id]=tr.x;textRects[e.id]={X:tr.x,Y:tr.y,Width:tr.width,Height:tr.height};}
            if(e.id.startsWith('case-')&&e.innerText.trim())textCases[e.id]=true;
          }
          for(const e of document.querySelectorAll('button[id],input[id],select[id],textarea[id]'))controlDisplays[e.id]=getComputedStyle(e).display;
          for(const tag of ['button','input','select','textarea']){
            const e=document.createElement(tag);document.body.append(e);standaloneControlDisplays[tag]=getComputedStyle(e).display;e.remove();
          }
          if(%(workbench)s){
            for(const [name,selector] of [['dialog-card','.dialog-card'],['dialog-actions','.actions']]){
              const e=document.querySelector(selector),r=e.getBoundingClientRect();boxes[name]={X:r.x,Y:r.y,Width:r.width,Height:r.height};
            }
          }
          const versions=await navigator.userAgentData.getHighEntropyValues(["fullVersionList"]);
          return JSON.stringify({boxes,text,textRects,textCases,controlDisplays,standaloneControlDisplays,images,fullVersionList:versions.fullVersionList,userAgent:navigator.userAgent,dpr:devicePixelRatio,width:innerWidth,height:innerHeight});
        })()''' % {
            'workbench': json.dumps(args.fixture == 'workbench'),
            'images': '[...document.querySelectorAll("img")].map(e=>{const r=e.getBoundingClientRect();return {X:r.x,Y:r.y,Width:r.width,Height:r.height}})' if args.fixture == 'workbench' else '[]',
        }
        result = json.loads(json.loads(run(browser + ['eval', '--stdin'], input=script)))
        if result['dpr'] != 1 or result['width'] != 1200 or result['height'] != 650:
            raise RuntimeError('Unexpected browser viewport/DPR')
        if args.update_browser:
            (RESOURCES/'chromium-layout.json').write_text(json.dumps(result['boxes'],indent=2)+'\n')
            (RESOURCES/'chromium-text.json').write_text(json.dumps(result['text'],indent=2)+'\n')
            if args.fixture == 'workbench':
                displays={'standalone':result['standaloneControlDisplays'],'workbench':result['controlDisplays']}
                (RESOURCES/'chromium-control-display.json').write_text(json.dumps(displays,indent=2)+'\n')
            if args.fixture == 'line-height':
                (RESOURCES/'chromium-text-vertical.json').write_text(json.dumps(result['textRects'],indent=2)+'\n')
        if not args.update_browser:
            baseline=json.loads((RESOURCES/'chromium-layout.json').read_text())
            if baseline.keys()!=result['boxes'].keys():raise RuntimeError('Browser baseline IDs changed')
            for name,rect in baseline.items():
                if any(abs(value-result['boxes'][name][axis])>0.1 for axis,value in rect.items()):
                    raise RuntimeError(f'Browser geometry changed: {name}; review before --update-browser')
            for name,x in json.loads((RESOURCES/'chromium-text.json').read_text()).items():
                if abs(result['text'][name]-x)>0.1:raise RuntimeError(f'Browser text metrics changed: {name}')
            if args.fixture == 'line-height':
                baseline=json.loads((RESOURCES/'chromium-text-vertical.json').read_text())
                if baseline.keys()!=result['textRects'].keys():raise RuntimeError('Browser vertical text baseline IDs changed')
                for name,rect in baseline.items():
                    if any(abs(rect[axis]-result['textRects'][name][axis])>0.1 for axis in ('X','Y','Width','Height')):
                        raise RuntimeError(f'Browser vertical text metrics changed: {name}; review before --update-browser')
            if args.fixture == 'workbench':
                displays=json.loads((RESOURCES/'chromium-control-display.json').read_text())
                current={'standalone':result['standaloneControlDisplays'],'workbench':result['controlDisplays']}
                if displays!=current:raise RuntimeError('Browser form-control display values changed; review before --update-browser')
        run(browser + ['screenshot', str(RESOURCES/'chromium.png')])
        with tempfile.TemporaryDirectory(prefix='onix-htmlcompat-') as work:
            binary = str(Path(work)/'htmlcompat')
            run(['go','build','-o',binary,'./tests/htmlcompat'])
            rasterizer=run([binary,'--raster-info'])
            xvfb = subprocess.Popen(['Xvfb','-displayfd','1','-screen','0','1200x650x24','-ac'],stdout=subprocess.PIPE,text=True)
            display = ':' + read_ready(xvfb)
            native = subprocess.Popen([binary,str(fixture)],cwd=ROOT,env={**os.environ,'DISPLAY':display},stdout=subprocess.PIPE,text=True)
            read_ready(native,'htmlcompat-ready')
            xwd = Path(work)/'native.xwd'
            run(['xwd','-display',display,'-root','-silent','-out',str(xwd)])
            raw=xwd.read_bytes(); header=struct.unpack('>25I',raw[:100])
            if (header[3],header[7],header[11],header[14],header[15],header[16]) != (24,0,32,16711680,65280,255):
                raise RuntimeError('Unexpected XWD pixel format')
            onix=Image.frombytes('RGB',(header[4],header[5]),raw[header[0]+header[19]*12:],'raw','BGRX',header[12],1)
            onix.save(RESOURCES/'onix.png')
        reference=Image.open(RESOURCES/'chromium.png').convert('RGB')
        diff=ImageChops.difference(reference,onix)
        diff.save(RESOURCES/'diff.png')
        def boundary_excess(left,right):
            low=right.filter(ImageFilter.MinFilter(3)); high=right.filter(ImageFilter.MaxFilter(3))
            return ImageChops.lighter(ImageChops.subtract(low,left),ImageChops.subtract(left,high))
        excess=ImageChops.lighter(boundary_excess(reference,onix),boundary_excess(onix,reference))
        excess.save(RESOURCES/'boundary-diff.png')
        cases={}
        for name,r in result['boxes'].items():
            if not name.startswith('case-'):continue
            area=diff.crop((int(r['X']),int(r['Y']),int(r['X']+r['Width']),int(r['Y']+r['Height'])))
            pixels=list(area.getdata())
            bounds=(int(r['X']),int(r['Y']),int(r['X']+r['Width']),int(r['Y']+r['Height']))
            edge_errors=sum(max(p)>16 for p in excess.crop(bounds).getdata())
            entry={'pixelsOver16':sum(max(p)>16 for p in pixels),'pixels':len(pixels),'meanAbsoluteRGB':ImageStat.Stat(area).mean,'boundaryPixelsOver16':edge_errors}
            probe_name='text-'+name[5:] if args.fixture == 'text' else name[5:]
            if probe_name.startswith('text-') and probe_name in result['boxes']:
                entry.update(text_ink_error(reference,onix,result['boxes'][probe_name]))
            if args.fixture == 'text':
                # Raw RGB error remains visible; this is a raster smoke-test budget, not pixel equality.
                entry['rasterPass']=max(entry['meanAbsoluteRGB']) <= 1.5
            elif name.startswith('case-text-') or result['textCases'].get(name):
                # Compare text ink geometry independently of grayscale/LCD
                # antialiasing. Raw and boundary differences remain in the
                # report as diagnostics.
                a=reference.crop(bounds).convert('L').point(lambda value:255 if value<90 else 0)
                b=onix.crop(bounds).convert('L').point(lambda value:255 if value<90 else 0)
                ink=ImageChops.lighter(ImageChops.subtract(a,b.filter(ImageFilter.MaxFilter(3))),ImageChops.subtract(b,a.filter(ImageFilter.MaxFilter(3))))
                entry['inkPixelsBeyond1px']=sum(value>0 for value in ink.getdata())
                entry['structuralPass']=entry['inkPixelsBeyond1px']==0
            else:
                entry['structuralPass']=edge_errors==0
            cases[name[5:]]=entry
        report={k:result[k] for k in ('userAgent','fullVersionList','dpr','width','height')}
        report['fontRasterizer']=rasterizer
        report['textRasterProfile']={'gamma':1.2,'effectiveGamma':76/64,'contrast':0.2,'horizontalSamples':4,'lcdFilterTaps':12}
        report['fixtureSHA256']=hashlib.sha256(fixture.read_bytes()).hexdigest()
        report['fontSHA256']=hashlib.sha256((ROOT/'testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf').read_bytes()).hexdigest()
        report['criteria']={'geometryToleranceCSSPixels':0.1,'boundaryRadiusPixels':1,'colorChannelTolerance':16,'textInkLuminanceThreshold':90,'pixelExact':False,'inkInteriorBackgroundDistance':12,'inkInteriorBoxInset':2}
        if args.fixture == 'text': report['criteria']['maxMeanChannelError'] = 1.5
        report['cases']=cases
        if args.fixture == 'workbench':
            from collections import Counter
            image_metrics=[]
            for rect in result.get('images',[]):
                bounds=(int(rect['X']),int(rect['Y']),int(rect['X']+rect['Width']),int(rect['Y']+rect['Height']))
                crop=onix.crop(bounds)
                background=Counter(crop.getdata()).most_common(1)[0][0] if crop.width and crop.height else (0,0,0)
                ink=sum(max(abs(pixel[channel]-background[channel]) for channel in range(3))>16 for pixel in crop.getdata())
                image_metrics.append({'rect':rect,'nativePixelsOver16':ink})
            report['images']={'count':len(image_metrics),'metrics':image_metrics,'allVisible':all(item['nativePixelsOver16']>0 for item in image_metrics)}
            if not report['images']['allVisible']:
                raise RuntimeError('Workbench image comparison failed; one or more SVG icons produced no native pixels')
        result_key = 'allRasterPass' if args.fixture == 'text' else 'allStructuralPass'
        case_key = 'rasterPass' if args.fixture == 'text' else 'structuralPass'
        report[result_key]=all(case[case_key] for case in cases.values())
        if args.fixture in ('line-height', 'workbench'):
            report[result_key]=None
            report['automatedAcceptance']=('go test ./ecs -run TestHTMLBrowserLineHeight'
                                           if args.fixture == 'line-height'
                                           else 'go test ./ecs -run TestHTMLBrowserWorkbench')
            report['visualComparison']='diagnostic only; system sans-serif raster metrics differ from Onix Noto Sans SC'
        (RESOURCES/'visual-report.json').write_text(json.dumps(report,indent=2)+'\n')
        print(json.dumps(report,indent=2))
        if report[result_key] is False:raise RuntimeError(f"Visual comparison failed; inspect {RESOURCES}/diff.png")
    finally:
        for process in (native,xvfb):
            if process is not None:
                process.terminate()
                try:process.wait(timeout=5)
                except subprocess.TimeoutExpired:process.kill();process.wait()
        run(browser+['close'])
        server.shutdown()


if __name__ == '__main__':
    main()
