package period

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func date(year int, month time.Month, day int) *time.Time {
	value := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &value
}

func TestNewRepeatKind(t *testing.T) {
	tests := []struct {
		value string
		want  error
	}{
		{"", nil},
		{"daily", nil},
		{"weekly", nil},
		{"monthly", nil},
		{"yearly", nil},
		{"every", nil},
		{"hourly", ErrRepeatKindInvalid},
		{"Monthly", ErrRepeatKindInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			kind, err := NewRepeatKind(tt.value)

			if kind != RepeatKind(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewRepeatKind(%q) = (%q, %v), want %v", tt.value, kind, err, tt.want)
			}
		})
	}
}

func TestRepeatValidate(t *testing.T) {
	tests := []struct {
		name   string
		repeat Repeat
		want   error
	}{
		{"none", NewRepeat(RepeatNone, 0, nil), nil},
		{"monthly forever", NewRepeat(RepeatMonthly, 0, nil), nil},
		{"weekly until a date", NewRepeat(RepeatWeekly, 0, date(2026, 6, 1)), nil},
		{"every two weeks", NewRepeat(RepeatEvery, 14*24*time.Hour, nil), nil},
		{"daily forever", NewRepeat(RepeatDaily, 0, nil), nil},
		{"unknown kind", NewRepeat("hourly", 0, nil), ErrRepeatKindInvalid},
		{"daily with a step", NewRepeat(RepeatDaily, time.Hour, nil), ErrRepeatEveryUnexpected},
		{"every without a step", NewRepeat(RepeatEvery, 0, nil), ErrRepeatEveryNotPositive},
		{"every with a negative step", NewRepeat(RepeatEvery, -time.Hour, nil), ErrRepeatEveryNotPositive},
		{"monthly with a step", NewRepeat(RepeatMonthly, time.Hour, nil), ErrRepeatEveryUnexpected},
		{"none with a step", NewRepeat(RepeatNone, time.Hour, nil), ErrRepeatEveryUnexpected},
		{"none until a date", NewRepeat(RepeatNone, 0, date(2026, 6, 1)), ErrRepeatUntilUnexpected},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.repeat.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if err != nil && !errors.Is(err, ErrRepeatInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrRepeatInvalid)
			}
		})
	}
}

func TestPeriodValidate(t *testing.T) {
	none := NewRepeat(RepeatNone, 0, nil)
	monthly := NewRepeat(RepeatMonthly, 0, nil)

	tests := []struct {
		name   string
		period Period
		want   error
	}{
		{"unbounded", NewPeriod(nil, nil, none), nil},
		{"from only", NewPeriod(date(2026, 1, 1), nil, none), nil},
		{"to only", NewPeriod(nil, date(2026, 1, 1), none), nil},
		{"one window", NewPeriod(date(2026, 1, 1), date(2026, 3, 1), none), nil},
		{"whole months", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), monthly), nil},
		{"first half of each month", NewPeriod(date(2026, 1, 1), date(2026, 1, 16), monthly), nil},
		{"whole months until a date", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), NewRepeat(RepeatMonthly, 0, date(2026, 6, 15))), nil},
		{"whole days", NewPeriod(date(2026, 1, 1), date(2026, 1, 2), NewRepeat(RepeatDaily, 0, nil)), nil},
		{"whole weeks", NewPeriod(date(2026, 1, 5), date(2026, 1, 12), NewRepeat(RepeatWeekly, 0, nil)), nil},
		{"whole years", NewPeriod(date(2026, 1, 1), date(2027, 1, 1), NewRepeat(RepeatYearly, 0, nil)), nil},
		{"every ten days", NewPeriod(date(2026, 1, 1), date(2026, 1, 11), NewRepeat(RepeatEvery, 10*24*time.Hour, nil)), nil},
		{"ends when it starts", NewPeriod(date(2026, 1, 1), date(2026, 1, 1), none), ErrPeriodEmpty},
		{"ends before it starts", NewPeriod(date(2026, 2, 1), date(2026, 1, 1), monthly), ErrPeriodEmpty},
		{"invalid repeat", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), NewRepeat(RepeatEvery, 0, nil)), ErrRepeatEveryNotPositive},
		{"repeats without an end", NewPeriod(date(2026, 1, 1), nil, monthly), ErrPeriodRepeatUnbounded},
		{"repeats without a start", NewPeriod(nil, date(2026, 1, 1), monthly), ErrPeriodRepeatUnbounded},
		{"window longer than a month", NewPeriod(date(2026, 1, 1), date(2026, 2, 2), monthly), ErrPeriodWindowTooLong},
		{"window longer than a day", NewPeriod(date(2026, 1, 1), date(2026, 1, 3), NewRepeat(RepeatDaily, 0, nil)), ErrPeriodWindowTooLong},
		{"window longer than a week", NewPeriod(date(2026, 1, 1), date(2026, 1, 9), NewRepeat(RepeatWeekly, 0, nil)), ErrPeriodWindowTooLong},
		{"window longer than the step", NewPeriod(date(2026, 1, 1), date(2026, 1, 12), NewRepeat(RepeatEvery, 10*24*time.Hour, nil)), ErrPeriodWindowTooLong},
		{"stops when it starts", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), NewRepeat(RepeatMonthly, 0, date(2026, 1, 1))), ErrPeriodUntilBeforeStart},
		{"times in different locations", NewPeriod(date(2026, 1, 1), new(time.Date(2026, 2, 1, 0, 0, 0, 0, time.FixedZone("EET", 2*60*60))), none), ErrPeriodLocationsDiffer},
		{"stops before it starts", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), NewRepeat(RepeatMonthly, 0, date(2025, 12, 1))), ErrPeriodUntilBeforeStart},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.period.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if err != nil && !errors.Is(err, ErrPeriodInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrPeriodInvalid)
			}
		})
	}
}

func TestPeriodWindowAt(t *testing.T) {
	none := NewRepeat(RepeatNone, 0, nil)
	monthly := NewRepeat(RepeatMonthly, 0, nil)

	tests := []struct {
		name     string
		period   Period
		at       *time.Time
		wantFrom *time.Time
		wantTo   *time.Time
		found    bool
	}{
		{"unbounded", NewPeriod(nil, nil, none), date(1900, 1, 1), nil, nil, true},
		{"after an open start", NewPeriod(nil, date(2026, 2, 1), none), date(2020, 1, 1), nil, date(2026, 2, 1), true},
		{"at an excluded end", NewPeriod(nil, date(2026, 2, 1), none), date(2026, 2, 1), nil, nil, false},
		{"at an included start", NewPeriod(date(2026, 1, 1), nil, none), date(2026, 1, 1), date(2026, 1, 1), nil, true},
		{"before the start", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), none), date(2025, 12, 31), nil, nil, false},
		{"first month", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), monthly), date(2026, 1, 20), date(2026, 1, 1), date(2026, 2, 1), true},
		{"later month", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), monthly), date(2027, 3, 31), date(2027, 3, 1), date(2027, 4, 1), true},
		{"where one month meets the next", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), monthly), date(2026, 5, 1), date(2026, 5, 1), date(2026, 6, 1), true},
		{"before a repeating start", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), monthly), date(2025, 12, 31), nil, nil, false},
		{"first half of a month", NewPeriod(date(2026, 1, 1), date(2026, 1, 16), monthly), date(2026, 4, 10), date(2026, 4, 1), date(2026, 4, 16), true},
		{"between short windows", NewPeriod(date(2026, 1, 1), date(2026, 1, 16), monthly), date(2026, 4, 20), nil, nil, false},
		{"end of a short month", NewPeriod(date(2026, 1, 31), date(2026, 2, 28), monthly), date(2026, 2, 28), date(2026, 2, 28), date(2026, 3, 28), true},
		{"end of a long month after a short one", NewPeriod(date(2026, 1, 31), date(2026, 2, 28), monthly), date(2026, 3, 31), date(2026, 3, 31), date(2026, 4, 28), true},
		{"leap day in a common year", NewPeriod(date(2024, 2, 29), date(2024, 3, 1), NewRepeat(RepeatYearly, 0, nil)), date(2025, 2, 28), date(2025, 2, 28), date(2025, 3, 1), true},
		{"day", NewPeriod(date(2026, 1, 1), date(2026, 1, 2), NewRepeat(RepeatDaily, 0, nil)), date(2026, 3, 4), date(2026, 3, 4), date(2026, 3, 5), true},
		{"day until a date", NewPeriod(date(2026, 1, 1), date(2026, 1, 2), NewRepeat(RepeatDaily, 0, date(2026, 1, 10))), date(2026, 1, 10), nil, nil, false},
		{"week", NewPeriod(date(2026, 1, 5), date(2026, 1, 12), NewRepeat(RepeatWeekly, 0, nil)), date(2026, 3, 4), date(2026, 3, 2), date(2026, 3, 9), true},
		{"every ten days", NewPeriod(date(2026, 1, 1), date(2026, 1, 6), NewRepeat(RepeatEvery, 10*24*time.Hour, nil)), date(2026, 1, 25), date(2026, 1, 21), date(2026, 1, 26), true},
		{"window cut by the end", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), NewRepeat(RepeatMonthly, 0, date(2026, 3, 15))), date(2026, 3, 10), date(2026, 3, 1), date(2026, 3, 15), true},
		{"at the end of the series", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), NewRepeat(RepeatMonthly, 0, date(2026, 3, 15))), date(2026, 3, 15), nil, nil, false},
		{"after the end of the series", NewPeriod(date(2026, 1, 1), date(2026, 2, 1), NewRepeat(RepeatMonthly, 0, date(2026, 3, 15))), date(2026, 5, 1), nil, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.period.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want a valid period", err)
			}

			window, found := tt.period.WindowAt(*tt.at)

			if found != tt.found || !sameTime(window.From, tt.wantFrom) || !sameTime(window.To, tt.wantTo) {
				t.Fatalf("WindowAt(%s) = (%s, %t), want ([%s, %s), %t)",
					tt.at, describe(window), found, describeTime(tt.wantFrom), describeTime(tt.wantTo), tt.found)
			}

			if contains := tt.period.Contains(*tt.at); contains != tt.found {
				t.Fatalf("Contains(%s) = %t, want %t", tt.at, contains, tt.found)
			}
		})
	}
}

// A weekly window keeps its wall-clock time when the clocks change, so the
// week that contains the change is an hour shorter.

func TestPeriodWindowAtAcrossDaylightSaving(t *testing.T) {
	riga, err := time.LoadLocation("Europe/Riga")

	if err != nil {
		t.Skipf("time zone data unavailable: %v", err)
	}

	from := time.Date(2026, 3, 23, 9, 0, 0, 0, riga)
	to := time.Date(2026, 3, 30, 9, 0, 0, 0, riga)
	period := NewPeriod(&from, &to, NewRepeat(RepeatWeekly, 0, nil))

	window, found := period.WindowAt(time.Date(2026, 4, 1, 12, 0, 0, 0, riga))

	if !found || window.From.Hour() != 9 || window.From.Day() != 30 || window.To.Day() != 6 || window.To.Hour() != 9 {
		t.Fatalf("WindowAt() = (%s, %t), want [Mar 30 09:00, Apr 6 09:00)", describe(window), found)
	}
}

// A daily window keeps its wall-clock time too, so the day the clocks move
// forward is an hour shorter.

func TestDailyWindowAcrossDaylightSaving(t *testing.T) {
	riga, err := time.LoadLocation("Europe/Riga")

	if err != nil {
		t.Skipf("time zone data unavailable: %v", err)
	}

	from := time.Date(2026, 3, 1, 8, 0, 0, 0, riga)
	to := time.Date(2026, 3, 1, 20, 0, 0, 0, riga)
	period := NewPeriod(&from, &to, NewRepeat(RepeatDaily, 0, nil))

	for _, tt := range []struct {
		at    time.Time
		day   int
		found bool
	}{
		{time.Date(2026, 3, 29, 8, 0, 0, 0, riga), 29, true},
		{time.Date(2026, 3, 30, 19, 59, 0, 0, riga), 30, true},
		{time.Date(2026, 3, 30, 7, 59, 0, 0, riga), 0, false},
		{time.Date(2026, 3, 30, 20, 0, 0, 0, riga), 0, false},
	} {
		window, found := period.WindowAt(tt.at)

		if found != tt.found || (found && (window.From.Day() != tt.day || window.From.Hour() != 8 || window.To.Hour() != 20)) {
			t.Fatalf("WindowAt(%s) = (%s, %t), want day %d 08:00–20:00, %t", tt.at, describe(window), found, tt.day, tt.found)
		}
	}
}

func TestAddMonths(t *testing.T) {
	tests := []struct {
		from   *time.Time
		months int
		want   *time.Time
	}{
		{date(2026, 1, 31), 1, date(2026, 2, 28)},
		{date(2026, 1, 31), 2, date(2026, 3, 31)},
		{date(2024, 1, 31), 1, date(2024, 2, 29)},
		{date(2026, 12, 15), 1, date(2027, 1, 15)},
		{date(2026, 3, 31), -1, date(2026, 2, 28)},
		{date(2024, 2, 29), 12, date(2025, 2, 28)},
	}

	for _, tt := range tests {
		if got := addMonths(*tt.from, tt.months); !got.Equal(*tt.want) {
			t.Fatalf("addMonths(%s, %d) = %s, want %s", tt.from, tt.months, got, tt.want)
		}
	}
}

func sameTime(a *time.Time, b *time.Time) bool {
	return (a == nil) == (b == nil) && (a == nil || a.Equal(*b))
}

func describe(window Window) string {
	return "[" + describeTime(window.From) + ", " + describeTime(window.To) + ")"
}

func describeTime(value *time.Time) string {
	result := "open"

	if value != nil {
		result = value.Format(time.DateTime)
	}

	return result
}
