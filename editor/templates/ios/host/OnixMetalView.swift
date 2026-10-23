import Metal
import QuartzCore
import UIKit

final class OnixMetalView: UIView, UITextViewDelegate {
    override class var layerClass: AnyClass { CAMetalLayer.self }

    private var displayLink: CADisplayLink?
    private var started = false
    private var surfaceInstalled = false
	private let textView = UITextView(frame: CGRect(x: 0, y: 0, width: 2, height: 2))
	private var textInputConfig = UInt64.max
	private var syncingTextSelection = false

    private var metalLayer: CAMetalLayer {
        layer as! CAMetalLayer
    }

    override init(frame: CGRect) {
        super.init(frame: frame)
        configure()
    }

    required init?(coder: NSCoder) {
        super.init(coder: coder)
        configure()
    }

	deinit {
		displayLink?.invalidate()
        if surfaceInstalled {
            OnixClearMetalLayer()
        }
    }

    private func configure() {
        isMultipleTouchEnabled = true
        metalLayer.device = MTLCreateSystemDefaultDevice()
        metalLayer.pixelFormat = .bgra8Unorm
        metalLayer.framebufferOnly = true
        let link = CADisplayLink(target: self, selector: #selector(drawFrame))
        link.add(to: .main, forMode: .common)
        displayLink = link
		textView.delegate = self
		textView.alpha = 0.01
		textView.autocorrectionType = .no
		textView.autocapitalizationType = .none
		textView.spellCheckingType = .no
		addSubview(textView)
		addGestureRecognizer(UILongPressGestureRecognizer(target: self, action: #selector(showTextEditMenu(_:))))
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        let scale = window?.screen.scale ?? UIScreen.main.scale
        contentScaleFactor = scale
        metalLayer.contentsScale = scale
        let width = max(Int(bounds.width * scale), 1)
        let height = max(Int(bounds.height * scale), 1)
        metalLayer.drawableSize = CGSize(width: width, height: height)
        OnixSetDevicePixelRatio(Float(scale))

        if !surfaceInstalled {
            let pointer = UInt(bitPattern: Unmanaged.passUnretained(metalLayer).toOpaque())
            OnixSetMetalLayer(pointer, Int32(width), Int32(height))
            surfaceInstalled = true
        }
        if !started {
            OnixStart(Int32(width), Int32(height))
            started = true
        } else {
            OnixResize(Int32(width), Int32(height))
        }
    }

    @objc private func drawFrame() {
        guard started, surfaceInstalled, window != nil else { return }
		OnixUpdate()
		syncTextInput()
    }

    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        dispatch(touches, OnixTouchStart)
    }

    override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent?) {
        dispatch(touches, OnixTouchMove)
    }

    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        dispatch(touches, OnixTouchEnd)
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        dispatch(touches, OnixTouchEnd)
    }

    private func dispatch(_ touches: Set<UITouch>, _ callback: (Float, Float) -> Void) {
        guard let touch = touches.first else { return }
        let point = touch.location(in: self)
        callback(Float(point.x), Float(point.y))
    }

	private func syncTextInput() {
		let config = UInt64(OnixTextInputConfig())
		let active = config & 1 != 0
		if config != textInputConfig {
			textInputConfig = config
			if active {
				textView.text = initialTextInputValue()
				let kind = (config >> 2) & 3
				textView.keyboardType = kind == 1 ? .decimalPad : (kind == 2 ? .numbersAndPunctuation : .default)
				textView.returnKeyType = config & 2 != 0 ? .default : .done
				textView.becomeFirstResponder()
			} else {
				textView.resignFirstResponder()
			}
		}
		guard active else { return }
		let start = utf16Offset(in: textView.text ?? "", codePoints: Int(OnixTextInputSelectionStart()))
		let end = utf16Offset(in: textView.text ?? "", codePoints: Int(OnixTextInputSelectionEnd()))
		let range = NSRange(location: min(start, end), length: abs(end - start))
		if textView.selectedRange != range {
			syncingTextSelection = true
			textView.selectedRange = range
			syncingTextSelection = false
		}
	}

	@objc private func showTextEditMenu(_ gesture: UILongPressGestureRecognizer) {
		guard gesture.state == .began, textInputConfig & 1 != 0 else { return }
		textView.becomeFirstResponder()
		let point = gesture.location(in: self)
		UIMenuController.shared.showMenu(from: textView, rect: CGRect(origin: point, size: CGSize(width: 1, height: 1)))
	}

	private func initialTextInputValue() -> String {
		let length = Int(OnixTextInputValue(nil, 0))
		guard length > 0 else { return "" }
		var buffer = [CChar](repeating: 0, count: length + 1)
		buffer.withUnsafeMutableBufferPointer { pointer in
			_ = OnixTextInputValue(pointer.baseAddress, Int32(pointer.count))
		}
		return String(cString: buffer)
	}

	func textViewDidChangeSelection(_ textView: UITextView) {
		guard !syncingTextSelection, textInputConfig & 1 != 0 else { return }
		let value = textView.text ?? ""
		let start = codePointOffset(in: value, utf16Offset: textView.selectedRange.location)
		let end = codePointOffset(in: value, utf16Offset: textView.selectedRange.location + textView.selectedRange.length)
		OnixTextSelection(textInputConfig, Int32(start), Int32(end))
	}

	func textView(_ textView: UITextView, shouldChangeTextIn range: NSRange, replacementText text: String) -> Bool {
		syncingTextSelection = true
		DispatchQueue.main.async { [weak self] in self?.syncingTextSelection = false }
		let value = textView.text ?? ""
		let start = codePointOffset(in: value, utf16Offset: range.location)
		let end = codePointOffset(in: value, utf16Offset: range.location + range.length)
		OnixTextSelection(textInputConfig, Int32(start), Int32(end))
		if text == "\n" && textInputConfig & 2 == 0 {
			OnixTextCommand(textInputConfig, 2)
			return false
		}
		if text.isEmpty {
			OnixTextCommand(textInputConfig, 1)
			return true
		}
		text.withCString { OnixTextInput(textInputConfig, $0) }
		return true
	}

	private func codePointOffset(in value: String, utf16Offset: Int) -> Int {
		let utf16 = value.utf16
		let index = utf16.index(utf16.startIndex, offsetBy: max(0, min(utf16Offset, utf16.count)))
		return String(decoding: utf16[..<index], as: UTF16.self).count
	}

	private func utf16Offset(in value: String, codePoints: Int) -> Int {
		let index = value.index(value.startIndex, offsetBy: max(0, min(codePoints, value.count)))
		return value[..<index].utf16.count
	}
}
