// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license in LICENSE.go-html.
// Adapted for Onix: source views, direct DOM construction and reusable storage.

package dom

import "strings"

func adjustAttributeNames(aa []htmlAttribute, nameMap map[string]string) {
	for i := range aa {
		if newName, ok := nameMap[aa[i].Key]; ok {
			aa[i].Key = newName
		}
	}
}

func htmlIntegrationPoint(n *Node) bool {
	if n.Type != ElementNode {
		return false
	}
	switch n.namespace {
	case "math":
		if n.Data == "annotation-xml" {
			for key, value := range n.Attributes {
				if key == "encoding" {
					if strings.EqualFold(value, "text/html") || strings.EqualFold(value, "application/xhtml+xml") {
						return true
					}
				}
			}
		}
	case "svg":
		switch n.Data {
		case "desc", "foreignObject", "title":
			return true
		}
	}
	return false
}

func mathMLTextIntegrationPoint(n *Node) bool {
	if n.namespace != "math" {
		return false
	}
	switch n.Data {
	case "mi", "mo", "mn", "ms", "mtext":
		return true
	}
	return false
}

var breakout = map[string]bool{
	"b":          true,
	"big":        true,
	"blockquote": true,
	"body":       true,
	"br":         true,
	"center":     true,
	"code":       true,
	"dd":         true,
	"div":        true,
	"dl":         true,
	"dt":         true,
	"em":         true,
	"embed":      true,
	"h1":         true,
	"h2":         true,
	"h3":         true,
	"h4":         true,
	"h5":         true,
	"h6":         true,
	"head":       true,
	"hr":         true,
	"i":          true,
	"img":        true,
	"li":         true,
	"listing":    true,
	"menu":       true,
	"meta":       true,
	"nobr":       true,
	"ol":         true,
	"p":          true,
	"pre":        true,
	"ruby":       true,
	"s":          true,
	"small":      true,
	"span":       true,
	"strong":     true,
	"strike":     true,
	"sub":        true,
	"sup":        true,
	"table":      true,
	"tt":         true,
	"u":          true,
	"ul":         true,
	"var":        true,
}

var svgTagNameAdjustments = map[string]string{
	"altglyph":            "altGlyph",
	"altglyphdef":         "altGlyphDef",
	"altglyphitem":        "altGlyphItem",
	"animatecolor":        "animateColor",
	"animatemotion":       "animateMotion",
	"animatetransform":    "animateTransform",
	"clippath":            "clipPath",
	"feblend":             "feBlend",
	"fecolormatrix":       "feColorMatrix",
	"fecomponenttransfer": "feComponentTransfer",
	"fecomposite":         "feComposite",
	"feconvolvematrix":    "feConvolveMatrix",
	"fediffuselighting":   "feDiffuseLighting",
	"fedisplacementmap":   "feDisplacementMap",
	"fedistantlight":      "feDistantLight",
	"feflood":             "feFlood",
	"fefunca":             "feFuncA",
	"fefuncb":             "feFuncB",
	"fefuncg":             "feFuncG",
	"fefuncr":             "feFuncR",
	"fegaussianblur":      "feGaussianBlur",
	"feimage":             "feImage",
	"femerge":             "feMerge",
	"femergenode":         "feMergeNode",
	"femorphology":        "feMorphology",
	"feoffset":            "feOffset",
	"fepointlight":        "fePointLight",
	"fespecularlighting":  "feSpecularLighting",
	"fespotlight":         "feSpotLight",
	"fetile":              "feTile",
	"feturbulence":        "feTurbulence",
	"foreignobject":       "foreignObject",
	"glyphref":            "glyphRef",
	"lineargradient":      "linearGradient",
	"radialgradient":      "radialGradient",
	"textpath":            "textPath",
}

var mathMLAttributeAdjustments = map[string]string{
	"definitionurl": "definitionURL",
}

var svgAttributeAdjustments = map[string]string{
	"attributename":       "attributeName",
	"attributetype":       "attributeType",
	"basefrequency":       "baseFrequency",
	"baseprofile":         "baseProfile",
	"calcmode":            "calcMode",
	"clippathunits":       "clipPathUnits",
	"diffuseconstant":     "diffuseConstant",
	"edgemode":            "edgeMode",
	"filterunits":         "filterUnits",
	"glyphref":            "glyphRef",
	"gradienttransform":   "gradientTransform",
	"gradientunits":       "gradientUnits",
	"kernelmatrix":        "kernelMatrix",
	"kernelunitlength":    "kernelUnitLength",
	"keypoints":           "keyPoints",
	"keysplines":          "keySplines",
	"keytimes":            "keyTimes",
	"lengthadjust":        "lengthAdjust",
	"limitingconeangle":   "limitingConeAngle",
	"markerheight":        "markerHeight",
	"markerunits":         "markerUnits",
	"markerwidth":         "markerWidth",
	"maskcontentunits":    "maskContentUnits",
	"maskunits":           "maskUnits",
	"numoctaves":          "numOctaves",
	"pathlength":          "pathLength",
	"patterncontentunits": "patternContentUnits",
	"patterntransform":    "patternTransform",
	"patternunits":        "patternUnits",
	"pointsatx":           "pointsAtX",
	"pointsaty":           "pointsAtY",
	"pointsatz":           "pointsAtZ",
	"preservealpha":       "preserveAlpha",
	"preserveaspectratio": "preserveAspectRatio",
	"primitiveunits":      "primitiveUnits",
	"refx":                "refX",
	"refy":                "refY",
	"repeatcount":         "repeatCount",
	"repeatdur":           "repeatDur",
	"requiredextensions":  "requiredExtensions",
	"requiredfeatures":    "requiredFeatures",
	"specularconstant":    "specularConstant",
	"specularexponent":    "specularExponent",
	"spreadmethod":        "spreadMethod",
	"startoffset":         "startOffset",
	"stddeviation":        "stdDeviation",
	"stitchtiles":         "stitchTiles",
	"surfacescale":        "surfaceScale",
	"systemlanguage":      "systemLanguage",
	"tablevalues":         "tableValues",
	"targetx":             "targetX",
	"targety":             "targetY",
	"textlength":          "textLength",
	"viewbox":             "viewBox",
	"viewtarget":          "viewTarget",
	"xchannelselector":    "xChannelSelector",
	"ychannelselector":    "yChannelSelector",
	"zoomandpan":          "zoomAndPan",
}

// Section 12.2.4.2 of the HTML5 specification says "The following elements
// have varying levels of special parsing rules".
// https://html.spec.whatwg.org/multipage/syntax.html#the-stack-of-open-elements
var isSpecialElementMap = map[string]bool{
	"address":    true,
	"applet":     true,
	"area":       true,
	"article":    true,
	"aside":      true,
	"base":       true,
	"basefont":   true,
	"bgsound":    true,
	"blockquote": true,
	"body":       true,
	"br":         true,
	"button":     true,
	"caption":    true,
	"center":     true,
	"col":        true,
	"colgroup":   true,
	"dd":         true,
	"details":    true,
	"dir":        true,
	"div":        true,
	"dl":         true,
	"dt":         true,
	"embed":      true,
	"fieldset":   true,
	"figcaption": true,
	"figure":     true,
	"footer":     true,
	"form":       true,
	"frame":      true,
	"frameset":   true,
	"h1":         true,
	"h2":         true,
	"h3":         true,
	"h4":         true,
	"h5":         true,
	"h6":         true,
	"head":       true,
	"header":     true,
	"hgroup":     true,
	"hr":         true,
	"html":       true,
	"iframe":     true,
	"img":        true,
	"input":      true,
	"keygen":     true, // "keygen" has been removed from the spec, but are kept here for backwards compatibility.
	"li":         true,
	"link":       true,
	"listing":    true,
	"main":       true,
	"marquee":    true,
	"menu":       true,
	"meta":       true,
	"nav":        true,
	"noembed":    true,
	"noframes":   true,
	"noscript":   true,
	"object":     true,
	"ol":         true,
	"p":          true,
	"param":      true,
	"plaintext":  true,
	"pre":        true,
	"script":     true,
	"section":    true,
	"select":     true,
	"source":     true,
	"style":      true,
	"summary":    true,
	"table":      true,
	"tbody":      true,
	"td":         true,
	"template":   true,
	"textarea":   true,
	"tfoot":      true,
	"th":         true,
	"thead":      true,
	"title":      true,
	"tr":         true,
	"track":      true,
	"ul":         true,
	"wbr":        true,
	"xmp":        true,
}

func isSpecialElement(element *Node) bool {
	switch element.namespace {
	case "", "html":
		return isSpecialElementMap[element.Data]
	case "math":
		switch element.Data {
		case "mi", "mo", "mn", "ms", "mtext", "annotation-xml":
			return true
		}
	case "svg":
		switch element.Data {
		case "foreignObject", "desc", "title":
			return true
		}
	}
	return false
}
