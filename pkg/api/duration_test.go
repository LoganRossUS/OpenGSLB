// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDuration_MarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		in   time.Duration
		want string
	}{
		{30 * time.Second, `"30s"`},
		{90 * time.Second, `"90s"`},
		{time.Hour, `"3600s"`},
		{500 * time.Millisecond, `"500ms"`},
		{1500 * time.Millisecond, `"1500ms"`},
		{0, `"0s"`},
		{1500 * time.Microsecond, `"1.5ms"`},
	} {
		got, err := json.Marshal(Duration(tc.in))
		if err != nil {
			t.Fatalf("%v: %v", tc.in, err)
		}
		if string(got) != tc.want {
			t.Errorf("Duration(%v) marshals to %s, want %s", tc.in, got, tc.want)
		}
		// Every output form is valid input and reads back as the same value.
		var back Duration
		if err := json.Unmarshal(got, &back); err != nil || time.Duration(back) != tc.in {
			t.Errorf("%s does not read back as %v: %v, %v", got, tc.in, time.Duration(back), err)
		}
	}
}

func TestDuration_UnmarshalJSON(t *testing.T) {
	for in, want := range map[string]time.Duration{
		`"30s"`:   30 * time.Second,
		`"500ms"`: 500 * time.Millisecond,
		`"1m30s"`: 90 * time.Second,
		`"1.5s"`:  1500 * time.Millisecond,
		`"2h"`:    2 * time.Hour,
		` "5s" `:  5 * time.Second,
	} {
		var d Duration
		if err := json.Unmarshal([]byte(in), &d); err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if time.Duration(d) != want {
			t.Errorf("%s = %v, want %v", in, time.Duration(d), want)
		}
	}

	// A number is what clients sent as seconds and the server read as
	// nanoseconds. No reading of it is safe, so it is refused and the message
	// shows the form to use.
	for _, in := range []string{`30`, `30000000000`, `5.5`, `0`, `-1`, `true`, `{}`, `[]`} {
		var d Duration
		err := json.Unmarshal([]byte(in), &d)
		if err == nil {
			t.Errorf("%s was accepted as %v", in, time.Duration(d))
			continue
		}
		if !strings.Contains(err.Error(), `"30s"`) {
			t.Errorf("%s: the error does not show the string form: %v", in, err)
		}
	}
	for _, in := range []string{`"30"`, `"abc"`, `""`, `"30 s"`, `"30sec"`} {
		var d Duration
		err := json.Unmarshal([]byte(in), &d)
		if err == nil {
			t.Errorf("%s was accepted as %v", in, time.Duration(d))
			continue
		}
		if !strings.Contains(err.Error(), "unit") {
			t.Errorf("%s: the error does not ask for a unit: %v", in, err)
		}
	}

	// null leaves a pointer field unset.
	var req struct {
		Interval *Duration `json:"interval"`
	}
	if err := json.Unmarshal([]byte(`{"interval":null}`), &req); err != nil || req.Interval != nil {
		t.Errorf("null: interval = %v, err = %v; want nil, nil", req.Interval, err)
	}
}
