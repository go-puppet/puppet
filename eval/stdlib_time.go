// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strconv"
	"strings"
	"time"
)

// registerStdlibTime installs the time/date functions. strftime mirrors Puppet
// core `strftime` (Ruby/C strftime directives); time mirrors puppetlabs-stdlib
// `stdlib::time` (current Unix epoch seconds).
func registerStdlibTime(e *Evaluator) {
	e.funcs["strftime"] = builtinStrftime
	e.funcs["time"] = builtinTime
	e.funcs["stdlib::time"] = builtinTime
}

// nowFunc is the clock, overridable in tests.
var nowFunc = time.Now

// builtinTime implements stdlib::time([timezone]): current epoch seconds. The
// optional timezone argument is accepted but ignored, matching upstream.
func builtinTime(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) > 1 {
		return nil, wrongArgs("time")
	}
	return nowFunc().UTC().Unix(), nil
}

// builtinStrftime implements strftime(format, [time], [timezone]).
//
//   - strftime(format)                     format the current time in UTC
//   - strftime(format, timezone: String)   format the current time in timezone
//   - strftime(format, time: Numeric)      format the given epoch time in UTC
//   - strftime(format, time, timezone)     format the given epoch time in tz
func builtinStrftime(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 3 {
		return nil, wrongArgs("strftime")
	}
	format, err := argStr(args, 0, "strftime")
	if err != nil {
		return nil, err
	}

	t := nowFunc().UTC()
	tz := ""
	if len(args) >= 2 {
		if secs, ok := asFloat(args[1]); ok {
			sec := int64(secs)
			nsec := int64((secs - float64(sec)) * 1e9)
			t = time.Unix(sec, nsec).UTC()
			if len(args) == 3 {
				tz, err = argStr(args, 2, "strftime")
				if err != nil {
					return nil, err
				}
			}
		} else {
			// Legacy Puppet-core form: strftime(format, timezone).
			if len(args) == 3 {
				return nil, &Error{Msg: "strftime(): second argument must be a Numeric time"}
			}
			tz, err = argStr(args, 1, "strftime")
			if err != nil {
				return nil, err
			}
		}
	}

	loc, err := resolveZone(tz)
	if err != nil {
		return nil, err
	}
	return strftime(t.In(loc), format), nil
}

// resolveZone resolves a timezone string to a *time.Location. "" and "default"
// mean UTC; "current" means the local zone; otherwise a location name
// ("America/Los_Angeles") or a numeric offset ("-0800", "+05:30").
func resolveZone(tz string) (*time.Location, error) {
	switch tz {
	case "", "default":
		return time.UTC, nil
	case "current":
		return time.Local, nil
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc, nil
	}
	if off, ok := parseOffset(tz); ok {
		return time.FixedZone(tz, off), nil
	}
	return nil, &Error{Msg: "strftime(): unknown timezone " + strconv.Quote(tz)}
}

// parseOffset parses "[+-]HH:MM" or "[+-]HHMM" into seconds east of UTC.
func parseOffset(s string) (int, bool) {
	if len(s) < 3 || (s[0] != '+' && s[0] != '-') {
		return 0, false
	}
	sign := 1
	if s[0] == '-' {
		sign = -1
	}
	body := strings.Replace(s[1:], ":", "", 1)
	if len(body) != 4 {
		return 0, false
	}
	hh, err1 := strconv.Atoi(body[:2])
	mm, err2 := strconv.Atoi(body[2:])
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return sign * (hh*3600 + mm*60), true
}

var (
	longMonths  = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	shortMonths = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	longDays    = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	shortDays   = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
)

// strftime expands a Ruby/C strftime format string against t.
func strftime(t time.Time, format string) string {
	var b strings.Builder
	i := 0
	for i < len(format) {
		if format[i] != '%' {
			b.WriteByte(format[i])
			i++
			continue
		}
		i++
		if i >= len(format) {
			b.WriteByte('%')
			break
		}
		// Parse flags and optional width.
		flags := ""
		for i < len(format) && strings.IndexByte("-_0^#:", format[i]) >= 0 {
			flags += string(format[i])
			i++
		}
		width := -1
		wstart := i
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			i++
		}
		if i > wstart {
			width, _ = strconv.Atoi(format[wstart:i])
		}
		if i >= len(format) {
			b.WriteByte('%')
			b.WriteString(flags)
			b.WriteString(format[wstart:i])
			break
		}
		b.WriteString(expandDirective(t, format[i], flags, width))
		i++
	}
	return b.String()
}

// expandDirective renders a single strftime conversion.
func expandDirective(t time.Time, d byte, flags string, width int) string {
	switch d {
	case '%':
		return "%"
	case 'n':
		return "\n"
	case 't':
		return "\t"
	case 'Y':
		return strconv.Itoa(t.Year())
	case 'C':
		return pad2(t.Year()/100, flags)
	case 'y':
		return pad2(t.Year()%100, flags)
	case 'm':
		return pad2(int(t.Month()), flags)
	case 'B':
		return applyCase(longMonths[t.Month()-1], flags)
	case 'b', 'h':
		return applyCase(shortMonths[t.Month()-1], flags)
	case 'd':
		return pad2(t.Day(), flags)
	case 'e':
		return space2(t.Day(), flags)
	case 'j':
		return pad(t.YearDay(), 3, flags)
	case 'H':
		return pad2(t.Hour(), flags)
	case 'k':
		return space2(t.Hour(), flags)
	case 'I':
		return pad2(hour12(t.Hour()), flags)
	case 'l':
		return space2(hour12(t.Hour()), flags)
	case 'M':
		return pad2(t.Minute(), flags)
	case 'S':
		return pad2(t.Second(), flags)
	case 'L':
		return fracSeconds(t.Nanosecond(), pickWidth(width, 3))
	case 'N':
		return fracSeconds(t.Nanosecond(), pickWidth(width, 9))
	case 'P':
		if t.Hour() < 12 {
			return "am"
		}
		return "pm"
	case 'p':
		if t.Hour() < 12 {
			return "AM"
		}
		return "PM"
	case 'A':
		return applyCase(longDays[int(t.Weekday())], flags)
	case 'a':
		return applyCase(shortDays[int(t.Weekday())], flags)
	case 'u':
		w := int(t.Weekday())
		if w == 0 {
			w = 7
		}
		return strconv.Itoa(w)
	case 'w':
		return strconv.Itoa(int(t.Weekday()))
	case 'z':
		return zoneOffset(t, flags)
	case 'Z':
		return t.Format("MST")
	case 'G':
		y, _ := t.ISOWeek()
		return strconv.Itoa(y)
	case 'g':
		y, _ := t.ISOWeek()
		return pad2(y%100, flags)
	case 'V':
		_, wk := t.ISOWeek()
		return pad2(wk, flags)
	case 'U':
		return pad2((t.YearDay()+6-int(t.Weekday()))/7, flags)
	case 'W':
		wdayMon := (int(t.Weekday()) + 6) % 7
		return pad2((t.YearDay()+6-wdayMon)/7, flags)
	case 's':
		return strconv.FormatInt(t.Unix(), 10)
	case 'c':
		return strftime(t, "%a %b %e %T %Y")
	case 'D', 'x':
		return strftime(t, "%m/%d/%y")
	case 'F':
		return strftime(t, "%Y-%m-%d")
	case 'v':
		return strftime(t, "%e-%^b-%Y")
	case 'X', 'T':
		return strftime(t, "%H:%M:%S")
	case 'r':
		return strftime(t, "%I:%M:%S %p")
	case 'R':
		return strftime(t, "%H:%M")
	default:
		return "%" + string(d)
	}
}

func hour12(h int) int {
	h %= 12
	if h == 0 {
		return 12
	}
	return h
}

func pickWidth(w, def int) int {
	if w < 0 {
		return def
	}
	return w
}

// fracSeconds renders nanoseconds as a width-digit fraction (truncated).
func fracSeconds(nsec, width int) string {
	s := pad(nsec, 9, "0")
	if width <= 9 {
		return s[:width]
	}
	return s + strings.Repeat("0", width-9)
}

// pad2 zero-pads n to width 2 honouring the "-" (none) and "_" (space) flags.
func pad2(n int, flags string) string { return pad(n, 2, flags) }

// space2 space-pads n to width 2 (the default for %e/%k/%l).
func space2(n int, flags string) string {
	if strings.Contains(flags, "0") {
		return pad(n, 2, "0")
	}
	return pad(n, 2, "_")
}

// pad renders n to the given width. flags select the pad style: "-" none,
// "_" spaces, otherwise zeros.
func pad(n, width int, flags string) string {
	s := strconv.Itoa(n)
	if strings.Contains(flags, "-") {
		return s
	}
	if len(s) >= width {
		return s
	}
	fill := byte('0')
	if strings.Contains(flags, "_") {
		fill = ' '
	}
	return strings.Repeat(string(fill), width-len(s)) + s
}

// applyCase applies the "^" (upcase) / "#" (upcase for names) flags.
func applyCase(s, flags string) string {
	if strings.Contains(flags, "^") || strings.Contains(flags, "#") {
		return strings.ToUpper(s)
	}
	return s
}

// zoneOffset renders %z. The ":" flags request colon-separated forms.
func zoneOffset(t time.Time, flags string) string {
	_, off := t.Zone()
	sign := "+"
	if off < 0 {
		sign = "-"
		off = -off
	}
	hh := off / 3600
	mm := (off % 3600) / 60
	ss := off % 60
	colons := strings.Count(flags, ":")
	switch colons {
	case 1:
		return sign + pad(hh, 2, "0") + ":" + pad(mm, 2, "0")
	case 2:
		return sign + pad(hh, 2, "0") + ":" + pad(mm, 2, "0") + ":" + pad(ss, 2, "0")
	default:
		return sign + pad(hh, 2, "0") + pad(mm, 2, "0")
	}
}
