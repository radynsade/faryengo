package period

import (
	"errors"
	"fmt"
	"time"
)

//
// Repeat kind
//

var ErrRepeatKindInvalid = errors.New("invalid repeat kind")

type RepeatKind string

const (
	RepeatNone    RepeatKind = ""
	RepeatDaily   RepeatKind = "daily"
	RepeatWeekly  RepeatKind = "weekly"
	RepeatMonthly RepeatKind = "monthly"
	RepeatYearly  RepeatKind = "yearly"
	RepeatEvery   RepeatKind = "every"
)

func NewRepeatKind(value string) (RepeatKind, error) {
	var (
		err  error
		kind RepeatKind
	)

	kind = RepeatKind(value)
	err = kind.Validate()
	return kind, err
}

func (k RepeatKind) Validate() error {
	var err error

	switch k {
	case RepeatNone, RepeatDaily, RepeatWeekly, RepeatMonthly, RepeatYearly, RepeatEvery:
	default:
		err = ErrRepeatKindInvalid
	}

	return err
}

//
// Repeat
//

// Every is the step of RepeatEvery and is zero for every other kind. Until
// ends the repetition: no window reaches it, so the last window may be cut
// short. A nil Until repeats forever, and a Period that does not repeat has
// none.

var (
	ErrRepeatInvalid          = errors.New("invalid repeat")
	ErrRepeatEveryNotPositive = errors.New("has a step that is not positive")
	ErrRepeatEveryUnexpected  = errors.New("has a step its kind does not use")
	ErrRepeatUntilUnexpected  = errors.New("has an end but does not repeat")
)

type Repeat struct {
	Kind  RepeatKind
	Every time.Duration
	Until *time.Time
}

func NewRepeat(kind RepeatKind, every time.Duration, until *time.Time) Repeat {
	return Repeat{
		Kind:  kind,
		Every: every,
		Until: until,
	}
}

func (r Repeat) Validate() error {
	err := r.Kind.Validate()

	if err == nil {
		if r.Kind == RepeatEvery && r.Every <= 0 {
			err = ErrRepeatEveryNotPositive
		} else if r.Kind != RepeatEvery && r.Every != 0 {
			err = ErrRepeatEveryUnexpected
		} else if r.Kind == RepeatNone && r.Until != nil {
			err = ErrRepeatUntilUnexpected
		}
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrRepeatInvalid, err)
	}

	return err
}

// Calendar steps are counted from the start of the series rather than from
// the previous window, and a month or year step keeps the day of the month
// unless the month is shorter, so a window that starts on January 31 starts
// on February 28, then on March 31. Steps follow the calendar of the time's
// location, so a daily or weekly window keeps its wall-clock time across a
// change of daylight saving time.

func (r Repeat) shift(t time.Time, steps int) time.Time {
	var shifted time.Time

	switch r.Kind {
	case RepeatDaily:
		shifted = t.AddDate(0, 0, steps)
	case RepeatWeekly:
		shifted = t.AddDate(0, 0, 7*steps)
	case RepeatMonthly:
		shifted = addMonths(t, steps)
	case RepeatYearly:
		shifted = addMonths(t, 12*steps)
	case RepeatEvery:
		shifted = t.Add(time.Duration(steps) * r.Every)
	default:
		shifted = t
	}

	return shifted
}

// The number of whole steps from start to t, for a t that is not before start.
// The estimate may be off by one where months differ in length or the clock
// changes, so it is corrected against the steps themselves.

func (r Repeat) stepsUntil(start time.Time, t time.Time) int {
	var steps int

	startYear, startMonth, _ := start.Date()
	year, month, _ := t.In(start.Location()).Date()

	switch r.Kind {
	case RepeatDaily:
		steps = int(t.Sub(start) / (24 * time.Hour))
	case RepeatWeekly:
		steps = int(t.Sub(start) / (7 * 24 * time.Hour))
	case RepeatMonthly:
		steps = (year-startYear)*12 + int(month-startMonth)
	case RepeatYearly:
		steps = year - startYear
	case RepeatEvery:
		steps = int(t.Sub(start) / r.Every)
	}

	for steps > 0 && r.shift(start, steps).After(t) {
		steps--
	}

	for !r.shift(start, steps+1).After(t) {
		steps++
	}

	return steps
}

//
// Window
//

// A window includes From and excludes To, so consecutive windows meet without
// overlapping. A nil bound leaves that side open.

type Window struct {
	From *time.Time
	To   *time.Time
}

func (w Window) Contains(t time.Time) bool {
	return (w.From == nil || !t.Before(*w.From)) && (w.To == nil || t.Before(*w.To))
}

//
// Period
//

// From and To bound the Period's first window; a nil bound leaves that side
// open. A repeating Period moves that window forward one step at a time, so
// it needs both bounds, and its window must end before the next one starts.
// Its repetition starts at From, so Until must come after it.

var (
	ErrPeriodInvalid          = errors.New("invalid period")
	ErrPeriodEmpty            = errors.New("does not end after it starts")
	ErrPeriodRepeatUnbounded  = errors.New("repeats a window without both bounds")
	ErrPeriodWindowTooLong    = errors.New("repeats a window longer than its step")
	ErrPeriodUntilBeforeStart = errors.New("stops repeating before it starts")
)

type Period struct {
	From   *time.Time
	To     *time.Time
	Repeat Repeat
}

func NewPeriod(from *time.Time, to *time.Time, repeat Repeat) Period {
	return Period{
		From:   from,
		To:     to,
		Repeat: repeat,
	}
}

func (p Period) Validate() error {
	var err error

	if p.From != nil && p.To != nil && !p.To.After(*p.From) {
		err = ErrPeriodEmpty
	} else {
		err = p.Repeat.Validate()
	}

	if err == nil && p.Repeat.Kind != RepeatNone {
		if p.From == nil || p.To == nil {
			err = ErrPeriodRepeatUnbounded
		} else if p.Repeat.shift(*p.From, 1).Before(*p.To) {
			err = ErrPeriodWindowTooLong
		} else if p.Repeat.Until != nil && !p.Repeat.Until.After(*p.From) {
			err = ErrPeriodUntilBeforeStart
		}
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrPeriodInvalid, err)
	}

	return err
}

// WindowAt finds the window that contains t. A Period that does not repeat
// has only its first window. A repeating Period's window ends at Until when
// Until comes first. The Period must be valid.

func (p Period) WindowAt(t time.Time) (Window, bool) {
	var (
		window Window
		found  bool
	)

	if p.Repeat.Kind == RepeatNone {
		window = Window{From: p.From, To: p.To}
		found = window.Contains(t)
	} else if !t.Before(*p.From) && (p.Repeat.Until == nil || t.Before(*p.Repeat.Until)) {
		steps := p.Repeat.stepsUntil(*p.From, t)
		from, to := p.Repeat.shift(*p.From, steps), p.Repeat.shift(*p.To, steps)

		if p.Repeat.Until != nil && p.Repeat.Until.Before(to) {
			to = *p.Repeat.Until
		}

		window = Window{From: &from, To: &to}
		found = window.Contains(t)
	}

	if !found {
		window = Window{}
	}

	return window, found
}

func (p Period) Contains(t time.Time) bool {
	_, found := p.WindowAt(t)

	return found
}

//
// Helpers
//

// The day of the month is kept unless the target month is shorter, in which
// case the month's last day is used; the time of day is kept.

func addMonths(t time.Time, months int) time.Time {
	year, month, day := t.Date()
	hour, minute, second := t.Clock()
	first := time.Date(year, month+time.Month(months), 1, 0, 0, 0, 0, t.Location())
	lastDay := first.AddDate(0, 1, -1).Day()

	return time.Date(first.Year(), first.Month(), min(day, lastDay), hour, minute, second, t.Nanosecond(), t.Location())
}
