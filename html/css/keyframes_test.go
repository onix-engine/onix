package css

import (
	"reflect"
	"testing"
)

func TestParseAndScopeKeyframes(t *testing.T) {
	input := `@media (min-width: 1px) { @keyframes "spin name" {
 from, 50% { transform: rotate(0deg); }
 120% { opacity: 0; }
 to { transform: rotate(360deg); opacity: .5 !important; }
 }} .spinner { animation: "spin name" 1s linear infinite; }`
	sheet, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.Rules) != 1 || len(sheet.Keyframes) != 1 {
		t.Fatalf("lost CSS rules: %+v", sheet)
	}
	k := sheet.Keyframes[0]
	if k.Name != "spin name" || len(k.Frames) != 2 || !reflect.DeepEqual(k.Frames[0].Offsets, []float64{0, .5}) || len(k.Conditions) != 1 {
		t.Fatalf("keyframes = %+v", k)
	}
	scoped, err := ScopeStylesheet(input, "component")
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := Parse(scoped)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reparsed.Keyframes, sheet.Keyframes) {
		t.Fatalf("scope changed keyframes: %s", scoped)
	}
	if reparsed.Rules[0].Selectors[0].Parts[0].Classes[1] != "component" {
		t.Fatal("ordinary selector not scoped")
	}
}

func TestKeyframeErrorRecoveryKeepsLaterRules(t *testing.T) {
	sheet, err := Parse(`@keyframes invalid { nope {opacity:0} -1%, 50% {opacity:1} 50% {color:red} } #after {width:10px}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.Rules) != 1 || len(sheet.Keyframes) != 1 || len(sheet.Keyframes[0].Frames) != 1 {
		t.Fatalf("recovery = %+v", sheet)
	}
}
