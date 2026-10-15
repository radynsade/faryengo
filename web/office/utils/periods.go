package utils

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/period"
	"github.com/radynsade/faryengo/pkg/forms"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/components"
	"github.com/radynsade/faryengo/web/office/templates/pages"
)

//
// Period form
//

// A period's first window is typed either as a custom range of local
// date-times, or as a whole day, month, or year, which the server turns into
// the range from its first moment to the first moment of the next one. Times
// are in the time zone the form names; the browser sends the device's zone
// with a new period, and an existing one keeps its own. A form without a zone
// uses UTC. A repeat of its own interval is typed as a whole number of
// minutes, hours, or days. Fields the chosen window or repeat does not use
// stay in the form but are ignored, so changing back keeps what was typed. A
// form without a window kind has a custom range.

const (
	periodTimeLayout     = "2006-01-02T15:04"
	periodTimeLayoutLong = "2006-01-02T15:04:05"
	maxPeriodEvery       = 99999
	maxPeriodYear        = 9999
)

// The kinds of a first window.

const (
	PeriodWindowCustom = "custom"
	PeriodWindowDay    = "day"
	PeriodWindowMonth  = "month"
	PeriodWindowYear   = "year"
)

var (
	ErrPeriodFromInvalid  = errors.New("period start is not a date and time")
	ErrPeriodToInvalid    = errors.New("period end is not a date and time")
	ErrPeriodUntilInvalid = errors.New("period repetition end is not a date and time")
	ErrPeriodEveryInvalid = errors.New("period step is not a whole number of minutes, hours, or days")
	ErrPeriodWindowKind   = errors.New("period window is not a custom range, day, month, or year")
	ErrPeriodDayMissing   = errors.New("period day is missing")
	ErrPeriodDayInvalid   = errors.New("period day is not a date")
	ErrPeriodMonthInvalid = errors.New("period month is not a month of the year")
	ErrPeriodYearMissing  = errors.New("period year is missing")
	ErrPeriodYearInvalid  = errors.New("period year is not a year")
	ErrPeriodRepeatUnfit  = errors.New("period repeat does not fit its whole window")
	periodFields          = []string{
		"period_repeat", "period_window", "period_from", "period_to", "period_day", "period_month", "period_year",
		"period_every", "period_every_unit", "period_until", "time_zone",
	}
	periodWindows = []string{PeriodWindowCustom, PeriodWindowDay, PeriodWindowMonth, PeriodWindowYear}

	// A whole day, month, or year repeats by calendar steps no shorter than
	// itself, so every window is again a whole day, month, or year: a whole
	// month cannot repeat every day or week. A repeat of its own interval
	// moves the window by a fixed duration, which leaves the calendar's
	// boundaries, so it fits only a custom range. The first repeat listed
	// is the one a window switches to when its repeat does not fit.
	periodWindowRepeats = map[string][]period.RepeatKind{
		PeriodWindowDay:   {period.RepeatDaily, period.RepeatNone, period.RepeatWeekly, period.RepeatMonthly, period.RepeatYearly},
		PeriodWindowMonth: {period.RepeatMonthly, period.RepeatNone, period.RepeatYearly},
		PeriodWindowYear:  {period.RepeatYearly, period.RepeatNone},
	}
	periodUnits = []struct {
		name string
		size time.Duration
	}{
		{"days", 24 * time.Hour},
		{"hours", time.Hour},
		{"minutes", time.Minute},
	}
)

type PeriodForm struct {
	Repeat    string `form:"period_repeat" validate:"maxbytes=16"`
	Window    string `form:"period_window" validate:"maxbytes=16"`
	From      string `form:"period_from" validate:"maxbytes=32"`
	To        string `form:"period_to" validate:"maxbytes=32"`
	Day       string `form:"period_day" validate:"maxbytes=16"`
	Month     string `form:"period_month" validate:"maxbytes=8"`
	Year      string `form:"period_year" validate:"maxbytes=8"`
	Every     string `form:"period_every" validate:"maxbytes=16"`
	EveryUnit string `form:"period_every_unit" validate:"maxbytes=16"`
	Until     string `form:"period_until" validate:"maxbytes=32"`
	TimeZone  string `form:"time_zone" validate:"maxbytes=64"`
}

func readPeriodForm(values url.Values) PeriodForm {
	return PeriodForm{
		Repeat:    values.Get("period_repeat"),
		Window:    values.Get("period_window"),
		From:      values.Get("period_from"),
		To:        values.Get("period_to"),
		Day:       values.Get("period_day"),
		Month:     values.Get("period_month"),
		Year:      values.Get("period_year"),
		Every:     values.Get("period_every"),
		EveryUnit: values.Get("period_every_unit"),
		Until:     values.Get("period_until"),
		TimeZone:  values.Get("time_zone"),
	}
}

// PeriodValues reads the form's period. A time zone the server does not know
// cannot come from the form's own script, so it makes the form malformed.
// Every other value that cannot be read is reported.

func PeriodValues(form PeriodForm) (input.PeriodInput, error) {
	var (
		result   input.PeriodInput
		location = time.UTC
		err      error
	)

	if form.TimeZone != "" {
		location, err = time.LoadLocation(form.TimeZone)

		if err != nil {
			err = errors.Join(forms.ErrInvalid, err)
		}
	}

	if err == nil {
		var windowErr, everyErr, untilErr error

		result.Repeat = form.Repeat
		result.From, result.To, windowErr = periodWindow(form, location)

		if windowErr == nil && !repeatFitsWindow(form.Window, period.RepeatKind(form.Repeat)) {
			windowErr = ErrPeriodRepeatUnfit
		}

		if form.Repeat == string(period.RepeatEvery) {
			result.Every, everyErr = parsePeriodEvery(form.Every, form.EveryUnit)
		}

		if form.Repeat != string(period.RepeatNone) {
			result.Until, untilErr = parsePeriodTime(form.Until, location, ErrPeriodUntilInvalid)
		}

		err = errors.Join(windowErr, everyErr, untilErr)
	}

	if err != nil {
		result = input.PeriodInput{}
	}

	return result, err
}

// A whole day, month, or year runs from its first moment to the first moment
// of the next one, so it keeps the calendar's length across a change of the
// clocks.

func periodWindow(form PeriodForm, location *time.Location) (*time.Time, *time.Time, error) {
	var (
		from, to *time.Time
		err      error
	)

	switch form.Window {
	case "", PeriodWindowCustom:
		var fromErr, toErr error

		from, fromErr = parsePeriodTime(form.From, location, ErrPeriodFromInvalid)
		to, toErr = parsePeriodTime(form.To, location, ErrPeriodToInvalid)
		err = errors.Join(fromErr, toErr)
	case PeriodWindowDay:
		var day time.Time

		if form.Day == "" {
			err = ErrPeriodDayMissing
		} else if day, err = time.ParseInLocation(time.DateOnly, form.Day, location); err != nil {
			err = errors.Join(ErrPeriodDayInvalid, err)
		} else {
			from, to = &day, new(day.AddDate(0, 0, 1))
		}
	case PeriodWindowMonth:
		year, yearErr := parsePeriodYear(form.Year)
		month, monthErr := strconv.Atoi(form.Month)

		if monthErr != nil || month < 1 || month > 12 {
			monthErr = ErrPeriodMonthInvalid
		}

		err = errors.Join(monthErr, yearErr)

		if err == nil {
			start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, location)
			from, to = &start, new(start.AddDate(0, 1, 0))
		}
	case PeriodWindowYear:
		var year int

		if year, err = parsePeriodYear(form.Year); err == nil {
			start := time.Date(year, time.January, 1, 0, 0, 0, 0, location)
			from, to = &start, new(start.AddDate(1, 0, 0))
		}
	default:
		err = ErrPeriodWindowKind
	}

	if err != nil {
		from, to = nil, nil
	}

	return from, to, err
}

func parsePeriodYear(value string) (int, error) {
	year, err := strconv.Atoi(value)

	if value == "" {
		err = ErrPeriodYearMissing
	} else if err != nil || year < 1 || year > maxPeriodYear {
		err = ErrPeriodYearInvalid
	}

	return year, err
}

func parsePeriodTime(value string, location *time.Location, invalid error) (*time.Time, error) {
	var (
		result *time.Time
		err    error
	)

	if value != "" {
		parsed, parseErr := time.ParseInLocation(periodTimeLayout, value, location)

		if parseErr != nil {
			parsed, parseErr = time.ParseInLocation(periodTimeLayoutLong, value, location)
		}

		if parseErr == nil {
			result = &parsed
		} else {
			err = errors.Join(invalid, parseErr)
		}
	}

	return result, err
}

func parsePeriodEvery(value string, unit string) (time.Duration, error) {
	var (
		every time.Duration
		err   = ErrPeriodEveryInvalid
	)

	count, countErr := strconv.Atoi(value)

	for _, candidate := range periodUnits {
		if countErr == nil && count >= 1 && count <= maxPeriodEvery && candidate.name == unit {
			every, err = time.Duration(count)*candidate.size, nil
		}
	}

	return every, err
}

// PeriodFieldErrors adds translated messages for a rejected period, keyed by
// form field name. A repeating period without both bounds is a problem with
// the bound that is missing. A whole day, month, or year that is longer than
// one repeat is a problem with the kind of window.

func PeriodFieldErrors(ctx context.Context, fields map[string]string, form PeriodForm, err error) {
	add := func(field string, messageID string) {
		if _, found := fields[field]; !found {
			fields[field] = officei18n.T(ctx, messageID)
		}
	}

	for _, check := range []struct {
		err       error
		field     string
		messageID string
	}{
		{ErrPeriodFromInvalid, "period_from", "validation.invalid"},
		{ErrPeriodToInvalid, "period_to", "validation.invalid"},
		{ErrPeriodUntilInvalid, "period_until", "validation.invalid"},
		{ErrPeriodWindowKind, "period_window", "validation.choose_one"},
		{ErrPeriodDayMissing, "period_day", "validation.required"},
		{ErrPeriodDayInvalid, "period_day", "validation.invalid"},
		{ErrPeriodMonthInvalid, "period_month", "validation.choose_one"},
		{ErrPeriodYearMissing, "period_year", "validation.required"},
		{ErrPeriodYearInvalid, "period_year", "periods.year_invalid"},
		{ErrPeriodEveryInvalid, "period_every", "periods.every_invalid"},
		{period.ErrRepeatEveryNotPositive, "period_every", "periods.every_invalid"},
		{period.ErrRepeatKindInvalid, "period_repeat", "validation.choose_one"},
		{ErrPeriodRepeatUnfit, "period_repeat", "periods.repeat_unfit"},
		{period.ErrPeriodEmpty, "period_to", "periods.end_before_start"},
		{period.ErrPeriodUntilBeforeStart, "period_until", "periods.until_before_start"},
	} {
		if errors.Is(err, check.err) {
			add(check.field, check.messageID)
		}
	}

	if errors.Is(err, period.ErrPeriodWindowTooLong) {
		if form.Window == "" || form.Window == PeriodWindowCustom {
			add("period_to", "periods.window_too_long")
		} else {
			add("period_window", "periods.window_too_long")
		}
	}

	if errors.Is(err, period.ErrPeriodRepeatUnbounded) {
		if form.From == "" {
			add("period_from", "periods.bounds_required")
		}

		if form.To == "" {
			add("period_to", "periods.bounds_required")
		}
	}
}

//
// Period selector
//

// The repeats and the units are offered in the order people reach for them.

// A repeat that does not fit the window is offered but disabled.

func periodRepeatOptions(ctx context.Context, window string) []components.SelectOption {
	kinds := []period.RepeatKind{
		period.RepeatNone,
		period.RepeatDaily,
		period.RepeatWeekly,
		period.RepeatMonthly,
		period.RepeatYearly,
		period.RepeatEvery,
	}
	options := make([]components.SelectOption, 0, len(kinds))

	for _, kind := range kinds {
		options = append(options, components.SelectOption{
			Value:    string(kind),
			Label:    officei18n.T(ctx, "periods.repeat_"+repeatKey(kind)),
			Disabled: !repeatFitsWindow(window, kind),
		})
	}

	return options
}

func periodUnitOptions(ctx context.Context) []components.SelectOption {
	options := make([]components.SelectOption, 0, len(periodUnits))

	for index := len(periodUnits) - 1; index >= 0; index-- {
		name := periodUnits[index].name
		options = append(options, components.SelectOption{Value: name, Label: officei18n.T(ctx, "periods.unit_"+name)})
	}

	return options
}

func periodPresets(ctx context.Context) []pages.PeriodPreset {
	presets := []string{"daily", "weekly", "monthly", "this_month"}
	result := make([]pages.PeriodPreset, 0, len(presets))

	for _, preset := range presets {
		result = append(result, pages.PeriodPreset{Value: preset, Label: officei18n.T(ctx, "periods.preset_"+preset)})
	}

	return result
}

func periodWindowOptions(ctx context.Context) []components.SelectOption {
	options := make([]components.SelectOption, 0, len(periodWindows))

	for _, window := range periodWindows {
		options = append(options, components.SelectOption{Value: window, Label: officei18n.T(ctx, "periods.window_"+window)})
	}

	return options
}

func periodMonthOptions(ctx context.Context) []components.SelectOption {
	options := make([]components.SelectOption, 0, 12)

	for month := time.January; month <= time.December; month++ {
		options = append(options, components.SelectOption{Value: strconv.Itoa(int(month)), Label: monthName(ctx, month)})
	}

	return options
}

// A new period applies at any time; its time zone is left for the browser to
// fill with the device's. A whole day, month, or year starts from today's, so
// choosing one needs no typing for the current one.

func NewPeriodSelectorProps(ctx context.Context) pages.PeriodSelectorProps {
	today := time.Now()

	return pages.PeriodSelectorProps{
		Repeats:      periodRepeatOptions(ctx, PeriodWindowCustom),
		RepeatFits:   periodRepeatFits(),
		Window:       PeriodWindowCustom,
		Windows:      periodWindowOptions(ctx),
		Day:          today.Format(time.DateOnly),
		Month:        strconv.Itoa(int(today.Month())),
		Months:       periodMonthOptions(ctx),
		Year:         strconv.Itoa(today.Year()),
		EveryUnit:    "days",
		EveryUnits:   periodUnitOptions(ctx),
		Presets:      periodPresets(ctx),
		TimeZoneHint: officei18n.T(ctx, "periods.device_time_zone"),
	}
}

// An existing period shows its times in its own time zone, which it keeps
// when saved. Its step is shown in the largest unit that divides it.

func PeriodSelectorProps(ctx context.Context, value period.Period) pages.PeriodSelectorProps {
	location := value.Location()
	props := NewPeriodSelectorProps(ctx)

	props.Repeat = string(value.Repeat.Kind)
	props.Window = periodWindowKind(value)
	props.Repeats = periodRepeatOptions(ctx, props.Window)
	props.From = formatPeriodInput(value.From, location)
	props.To = formatPeriodInput(value.To, location)
	props.Until = formatPeriodInput(value.Repeat.Until, location)
	props.TimeZone = location.String()
	props.TimeZoneHint = officei18n.T(ctx, "periods.time_zone", map[string]any{"Zone": location.String()})

	if value.From != nil {
		start := value.From.In(location)
		props.Day = start.Format(time.DateOnly)
		props.Month = strconv.Itoa(int(start.Month()))
		props.Year = strconv.Itoa(start.Year())
	}

	if value.Repeat.Kind == period.RepeatEvery {
		props.Every, props.EveryUnit = everyParts(value.Repeat.Every)
	}

	return props
}

// A rejected form shows the values that were submitted.

func SubmittedPeriodSelectorProps(ctx context.Context, props pages.PeriodSelectorProps, form PeriodForm) pages.PeriodSelectorProps {
	props.Repeat, props.From, props.To, props.Until = form.Repeat, form.From, form.To, form.Until
	props.Day, props.Month, props.Year = form.Day, form.Month, form.Year
	props.Every, props.EveryUnit = form.Every, form.EveryUnit
	props.Window = form.Window

	if props.Window == "" {
		props.Window = PeriodWindowCustom
	}

	props.Repeats = periodRepeatOptions(ctx, props.Window)

	if form.TimeZone != "" {
		props.TimeZone = form.TimeZone
		props.TimeZoneHint = officei18n.T(ctx, "periods.time_zone", map[string]any{"Zone": form.TimeZone})
	}

	return props
}

//
// Period summary
//

// PeriodSummary describes a period in one line. Times are written in the
// period's time zone, as dates alone when every time is at midnight; a whole
// day, month, or year is named as such.

func PeriodSummary(ctx context.Context, value period.Period) string {
	layout := summaryLayout(value)
	from := formatPeriodSummaryTime(value.From, layout)
	to := formatPeriodSummaryTime(value.To, layout)
	var summary string

	switch {
	case value.Repeat.Kind != period.RepeatNone && value.Repeat.Until != nil:
		summary = officei18n.T(ctx, "periods.summary_repeating_until", map[string]any{
			"Repeat": repeatSummary(ctx, value.Repeat),
			"Window": windowSummary(ctx, value, from, to),
			"Until":  formatPeriodSummaryTime(value.Repeat.Until, layout),
		})
	case value.Repeat.Kind != period.RepeatNone:
		summary = officei18n.T(ctx, "periods.summary_repeating", map[string]any{
			"Repeat": repeatSummary(ctx, value.Repeat),
			"Window": windowSummary(ctx, value, from, to),
		})
	case value.From != nil && value.To != nil:
		summary = windowSummary(ctx, value, from, to)
	case value.From != nil:
		summary = officei18n.T(ctx, "periods.summary_from", map[string]any{"From": from})
	case value.To != nil:
		summary = officei18n.T(ctx, "periods.summary_before", map[string]any{"To": to})
	default:
		summary = officei18n.T(ctx, "periods.summary_any_time")
	}

	return summary
}

func windowSummary(ctx context.Context, value period.Period, from string, to string) string {
	var summary string

	start := value.From.In(value.Location())

	switch periodWindowKind(value) {
	case PeriodWindowDay:
		summary = start.Format(time.DateOnly)
	case PeriodWindowMonth:
		summary = officei18n.T(ctx, "periods.month_year", map[string]any{
			"Month": monthName(ctx, start.Month()),
			"Year":  start.Year(),
		})
	case PeriodWindowYear:
		summary = strconv.Itoa(start.Year())
	default:
		summary = officei18n.T(ctx, "periods.summary_window", map[string]any{"From": from, "To": to})
	}

	return summary
}

func repeatSummary(ctx context.Context, repeat period.Repeat) string {
	summary := officei18n.T(ctx, "periods.repeat_"+repeatKey(repeat.Kind))

	if repeat.Kind == period.RepeatEvery {
		count, unit := everyParts(repeat.Every)
		number, _ := strconv.Atoi(count)
		summary = officei18n.Count(ctx, "periods.every_"+unit, number)
	}

	return summary
}

//
// Helpers
//

func repeatFitsWindow(window string, repeat period.RepeatKind) bool {
	repeats, limited := periodWindowRepeats[window]

	return !limited || slices.Contains(repeats, repeat)
}

// The script learns the rule from the server: for each whole window, the
// repeats that fit it, the one to switch to first.

func periodRepeatFits() string {
	fits := make(map[string][]string, len(periodWindowRepeats))

	for window, repeats := range periodWindowRepeats {
		for _, repeat := range repeats {
			fits[window] = append(fits[window], string(repeat))
		}
	}

	encoded, _ := json.Marshal(fits)

	return string(encoded)
}

// A window that starts at the first moment of a day, month, or year and ends
// at the first moment of the next one is that whole day, month, or year, in
// the period's calendar; any other window, or one with an open side, is a
// custom range. So is a whole window whose repeat does not fit it, such as one
// saved before the rule, so it can be edited and saved again.

func periodWindowKind(value period.Period) string {
	kind := PeriodWindowCustom

	if value.From != nil && value.To != nil {
		location := value.Location()
		start, end := value.From.In(location), value.To.In(location)
		midnight := start.Equal(time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, location))

		switch {
		case midnight && start.YearDay() == 1 && end.Equal(start.AddDate(1, 0, 0)):
			kind = PeriodWindowYear
		case midnight && start.Day() == 1 && end.Equal(start.AddDate(0, 1, 0)):
			kind = PeriodWindowMonth
		case midnight && end.Equal(start.AddDate(0, 0, 1)):
			kind = PeriodWindowDay
		}

		if !repeatFitsWindow(kind, value.Repeat.Kind) {
			kind = PeriodWindowCustom
		}
	}

	return kind
}

func monthName(ctx context.Context, month time.Month) string {
	return officei18n.T(ctx, "periods.month_"+strconv.Itoa(int(month)))
}

func repeatKey(kind period.RepeatKind) string {
	key := string(kind)

	if kind == period.RepeatNone {
		key = "none"
	}

	return key
}

// A step that is not a whole number of minutes, which the form cannot type,
// is shown in whole minutes, rounded down.

func everyParts(every time.Duration) (string, string) {
	count, unit := int64(every/time.Minute), "minutes"

	for index := len(periodUnits) - 1; index >= 0; index-- {
		candidate := periodUnits[index]

		if every >= candidate.size && every%candidate.size == 0 {
			count, unit = int64(every/candidate.size), candidate.name
		}
	}

	return strconv.FormatInt(count, 10), unit
}

func formatPeriodInput(value *time.Time, location *time.Location) string {
	formatted := ""

	if value != nil {
		formatted = value.In(location).Format(periodTimeLayout)
	}

	return formatted
}

func summaryLayout(value period.Period) string {
	layout := time.DateOnly

	for _, moment := range []*time.Time{value.From, value.To, value.Repeat.Until} {
		if moment != nil && (moment.Hour() != 0 || moment.Minute() != 0) {
			layout = "2006-01-02 15:04"
		}
	}

	return layout
}

func formatPeriodSummaryTime(value *time.Time, layout string) string {
	formatted := ""

	if value != nil {
		formatted = value.Format(layout)
	}

	return formatted
}
