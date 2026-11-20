// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Duration is a time span on the wire of the API. It is written as a string
// in Go duration syntax ("30s", "500ms", "1m30s"), the form the configuration
// file uses. A bare JSON number is refused: earlier versions decoded it as
// nanoseconds while clients meant seconds, so no reading of it is safe.
type Duration time.Duration

// MarshalJSON writes the duration in the shortest single-unit form: whole
// seconds as "30s", otherwise whole milliseconds as "1500ms", otherwise Go's
// own form. All of them are valid input.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// String returns the wire form of the duration.
func (d Duration) String() string {
	v := time.Duration(d)
	switch {
	case v%time.Second == 0:
		return strconv.FormatInt(int64(v/time.Second), 10) + "s"
	case v%time.Millisecond == 0:
		return strconv.FormatInt(int64(v/time.Millisecond), 10) + "ms"
	default:
		return v.String()
	}
}

// UnmarshalJSON reads a duration string and refuses everything else.
func (d *Duration) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '"' {
		return fmt.Errorf(`a duration must be a string with a unit, such as "30s" or "500ms"; got %s, which is ambiguous (seconds or nanoseconds) and is not accepted`, data)
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	v, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf(`%q is not a duration; use a number with a unit, such as "30s" or "500ms"`, text)
	}
	*d = Duration(v)
	return nil
}
