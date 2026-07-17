package adb

import "testing"

func TestStatusBarOptionsValidate(t *testing.T) {
	i := func(n int) *int { return &n }

	valid := []StatusBarOptions{
		{},                                 // all defaults
		{Clock: "0930"},                    // clock ok
		{Battery: i(0)}, {Battery: i(100)}, // battery bounds
		{MobileLevel: i(0)}, {MobileLevel: i(4)},
		{NetworkType: "wifi"}, {NetworkType: "mobile"}, {NetworkType: "none"},
		{NetworkType: "mobile", MobileLevel: i(2), DataType: "lte", Carrier: "X"},
	}
	for _, o := range valid {
		if err := o.validate(); err != nil {
			t.Errorf("validate(%+v) unexpected error: %v", o, err)
		}
	}

	invalid := []StatusBarOptions{
		{Clock: "930"},   // too short
		{Clock: "09:30"}, // non-digits
		{Battery: i(-1)}, {Battery: i(101)},
		{MobileLevel: i(-1)}, {MobileLevel: i(5)},
		{NetworkType: "cellular"}, // unknown type
	}
	for _, o := range invalid {
		if err := o.validate(); err == nil {
			t.Errorf("validate(%+v) expected error, got nil", o)
		}
	}
}
