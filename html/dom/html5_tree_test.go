package dom

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

func TestParseHTML5TreeConstruction(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{`<svg><foreignObject><template><div>inside</div></template><p>after`, `(svg)(foreignObject)(template)(div)"inside"(/)(/)(p)"after"(/)(/)(/)`},
		{"<p>one<p>two<table><tr><td>A<td>B", "(p)\"one\"(/)(p)\"two\"(/)(table)(tbody)(tr)(td)\"A\"(/)(td)\"B\"(/)(/)(/)(/)"},
		{"<b class=x><p>bold<i>italic</b>after</i>", "(b class=\"x\")(/)(p)(b class=\"x\")\"bold\"(i)\"italic\"(/)(/)(i)\"after\"(/)(/)"},
		{"<b><i>one</b>two</i>", "(b)(i)\"one\"(/)(/)(i)\"two\"(/)"},
		{"<a href=first>one<a href=second>two</a>three", "(a href=\"first\")\"one\"(/)(a href=\"second\")\"two\"(/)\"three\""},
		{"<table>before<tr><td>A<td>B</table>after", "\"before\"(table)(tbody)(tr)(td)\"A\"(/)(td)\"B\"(/)(/)(/)(/)\"after\""},
		{"<table><col><tr><td>cell</table>", "(table)(colgroup)(col)(/)(/)(tbody)(tr)(td)\"cell\"(/)(/)(/)(/)"},
		{"<table><div>outside</div><tr><td>inside", "(div)\"outside\"(/)(table)(tbody)(tr)(td)\"inside\"(/)(/)(/)(/)"},
		{"<div><table><svg><foreignObject><select><table><s>", "(div)(svg)(foreignObject)(select)(/)(/)(/)(table)(/)(s)(/)(table)(/)(/)"},
		{"<ul><li>one<li>two</ul><dl><dt>term<dd>definition<dt>next", "(ul)(li)\"one\"(/)(li)\"two\"(/)(/)(dl)(dt)\"term\"(/)(dd)\"definition\"(/)(dt)\"next\"(/)(/)"},
		{"<select><optgroup><option>One<option selected>Two</select>", "(select)(optgroup)(option)\"One\"(/)(option selected=\"\")\"Two\"(/)(/)(/)"},
		{"<form id=first><form id=second><input></form><input>", "(form id=\"first\")(input)(/)(/)(input)(/)"},
		{"<template><table><tr><td>cell</template><p>after", "(template)(table)(tbody)(tr)(td)\"cell\"(/)(/)(/)(/)(/)(p)\"after\"(/)"},
		{"<svg viewBox=\"0 0 10 10\"><foreignObject><p>HTML</p></foreignObject><path xlink:href=\"#x\"/></svg>", "(svg viewBox=\"0 0 10 10\")(foreignObject)(p)\"HTML\"(/)(/)(path xlink:href=\"#x\")(/)(/)"},
		{"<math definitionurl=x><mi><b>x</b></mi></math>", "(math definitionURL=\"x\")(mi)(b)\"x\"(/)(/)(/)"},
		{"<svg><![CDATA[A < B &amp; C]]><text>tail</text></svg>", "(svg)\"A < B & C\"(text)\"tail\"(/)(/)"},
		{"<script><!--<script></script>--></script><p>after", "(script)\"<!--<script></script>-->\"(/)(p)\"after\"(/)"},
		{"<style>a>b { content: \"&amp;\" }</style><p>after", "(style)\"a>b { content: \\\"&amp;\\\" }\"(/)(p)\"after\"(/)"},
		{"<textarea>\nA &amp; B <b>literal</b></textarea>", "(textarea)\"A & B <b>literal</b>\"(/)"},
		{"<pre><!--comment-->\nA</pre>", "(pre)\"\\nA\"(/)"},
		{"<pre>\nA\nB</pre>", "(pre)\"A\\nB\"(/)"},
		{"<!doctype html><html><body><p>body</body>tail</html>", "(html)(head)(/)(body)(p)\"bodytail\"(/)(/)(/)"},
		{"<!--leading--><head><title>Title &amp; more</title></head><p>body", "(html)(head)(title)\"Title & more\"(/)(/)(body)(p)\"body\"(/)(/)(/)"},
		{"<body><div A=first a=second data-v=&notit;>&notit; &#0A</div>", "(html)(head)(/)(body)(div a=\"first\" data-v=\"&notit;\")\"¬it; �A\"(/)(/)(/)"},
		{"<!DOCTYPE html PUBLIC \"-//W3C//DTD HTML 4.01 Transitional//EN\"><p>one<table><tr><td>two", "(html)(head)(/)(body)(p)\"one\"(table)(tbody)(tr)(td)\"two\"(/)(/)(/)(/)(/)(/)(/)"},
		{"<!doctype html><frameset><frame src=x></frameset>", "(html)(head)(/)(frameset)(frame src=\"x\")(/)(/)(/)"},
		{"<div/><span>inside</span></div>", "(div)(span)\"inside\"(/)(/)"},
		{"A<!-- gap -->B\x00C", "\"ABC\""},
	} {
		root, err := Parse(test.source)
		if err != nil {
			t.Fatal(err)
		}
		if got := htmlTreeShape(root); got != test.want {
			t.Errorf("source %q: got %s; want %s", test.source, got, test.want)
		}
	}
}

func htmlTreeShape(root *Node) string {
	var b strings.Builder
	var visit func(*Node)
	visit = func(n *Node) {
		switch n.Type {
		case ElementNode:
			fmt.Fprintf(&b, "(%s", n.Data)
			keys := make([]string, 0, len(n.Attributes))
			for k := range n.Attributes {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, " %s=%q", k, n.Attributes[k])
			}
			b.WriteByte(')')
			for _, c := range n.Children {
				visit(c)
			}
			b.WriteString("(/)")
		case TextNode:
			fmt.Fprintf(&b, "%q", n.Data)
		}
	}
	for _, child := range root.Children {
		visit(child)
	}
	return b.String()
}
