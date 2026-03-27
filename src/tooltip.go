package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// pangoVisibleLength calculates the visible text length by stripping Pango markup tags
func pangoVisibleLength(s string) int {
	// Remove all <span ...> and </span> tags
	re := regexp.MustCompile(`<[^>]+>`)
	visible := re.ReplaceAllString(s, "")
	// Count runes to handle emojis properly
	return len([]rune(visible))
}

// RunTooltipMode outputs a Pango markup version of the calendar for Waybar tooltips
func RunTooltipMode() {
	year, month, day := time.Now().Date()

	// Load configuration
	_, _, showHolidays, _, _, _ := loadDisplayConfig()
	today, _, headings, text, weekends, _ := getPalette()

	// Load calendars and events (match main app logic)
	calendars, vdirEvents := detectVdirsyncerCalendars()
	var events []Event
	if len(calendars) == 0 {
		calendars = loadCalendars()
		events = loadEvents(calendars)
	} else {
		calendars = mergeCalendarConfigs(calendars)
		events = vdirEvents
	}

	// Load holidays if enabled
	var holidays []Event
	if showHolidays {
		holidays = generateUSHolidays(year)
	}

	// Build calendar output
	// Get upcoming events (next 3) - build these first to calculate max width
	todayDate := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	upcomingEvents := getUpcomingEventsForTooltip(todayDate, events, holidays, showHolidays, 3)

	// Build upcoming events lines and calculate max width
	var eventLines []string
	maxEventWidth := 0

	if len(upcomingEvents) > 0 {
		headerLine := fmt.Sprintf("<span foreground=\"%s\"><b>Upcoming Events</b></span>", headings)
		eventLines = append(eventLines, headerLine)
		if w := pangoVisibleLength(headerLine); w > maxEventWidth {
			maxEventWidth = w
		}

		for _, event := range upcomingEvents {
			// Get calendar color
			calColor := string(text)
			for _, cal := range calendars {
				if cal.Name == event.CalendarName {
					calColor = string(cal.Color)
					break
				}
			}

			// Time string
			timeStr := "All-day"
			if !event.AllDay && event.Time != "" {
				timeStr = event.Time
			}

			// Free/Busy indicator
			fbStr := ""
			if event.FreeBusy == StatusFree {
				fbStr = " <span foreground=\"#a6e3a1\">(Free)</span>"
			}

			// Holiday emoji
			prefix := ""
			if event.CalendarName == "holidays" {
				prefix = "🎉 "
			}

			// Date string for upcoming events
			dateStr := event.Date.Format("Jan 02")

			line := fmt.Sprintf("<span foreground=\"%s\">●</span> <span foreground=\"%s\">%s %s</span> %s%s%s",
				calColor, headings, dateStr, timeStr, prefix, event.Title, fbStr)
			eventLines = append(eventLines, line)
			if w := pangoVisibleLength(line); w > maxEventWidth {
				maxEventWidth = w
			}
		}
	} else {
		line := fmt.Sprintf("<span foreground=\"%s\" style=\"italic\">No upcoming events</span>", text)
		eventLines = append(eventLines, line)
		if w := pangoVisibleLength(line); w > maxEventWidth {
			maxEventWidth = w
		}
	}

	// Calculate padding to center the 21-char calendar within the max event width
	calendarWidth := 21
	calendarPadding := ""
	if maxEventWidth > calendarWidth {
		padAmount := (maxEventWidth - calendarWidth) / 2
		calendarPadding = strings.Repeat(" ", padAmount)
	}

	var output strings.Builder

	// Title - centered over the calendar grid
	titleText := fmt.Sprintf("%s %d", month.String(), year)
	titlePadding := (calendarWidth - len(titleText)) / 2
	if titlePadding < 0 {
		titlePadding = 0
	}
	output.WriteString(fmt.Sprintf("<span foreground=\"%s\"><b>%s%s</b></span>\n\n",
		text, calendarPadding+strings.Repeat(" ", titlePadding), titleText))

	// Weekday headers
	headers := []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
	output.WriteString(calendarPadding)
	for i, header := range headers {
		color := headings
		if i == 0 || i == 6 {
			color = weekends
		}
		output.WriteString(fmt.Sprintf("<span foreground=\"%s\"><i>%s</i></span> ", color, header))
	}
	output.WriteString("\n")

	// Get month info
	firstDay := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	firstWeekday := int(firstDay.Weekday())
	lastDay := firstDay.AddDate(0, 1, -1).Day()

	// Build calendar grid
	currWeekDay := firstWeekday

	// Leading spaces (with calendar padding for first row)
	output.WriteString(calendarPadding)
	for i := 0; i < currWeekDay; i++ {
		output.WriteString("   ")
	}

	// Days
	for d := 1; d <= lastDay; d++ {
		if currWeekDay%7 == 0 && d != 1 {
			output.WriteString("\n" + calendarPadding)
		}

		dayStr := fmt.Sprintf("%2d", d)
		isToday := d == day
		isWeekend := currWeekDay%7 == 0 || currWeekDay%7 == 6
		eventCal := getEventCalendarOnDayForTooltip(d, month, year, events, holidays, calendars, showHolidays)

		if isToday {
			// Today - highlighted with background
			output.WriteString(fmt.Sprintf("<span background=\"%s\" foreground=\"%s\"><b>%s</b></span> ",
				today, "#1e1e2e", dayStr))
		} else if eventCal != nil {
			// Day with event - use calendar color
			output.WriteString(fmt.Sprintf("<span foreground=\"%s\"><u>%s</u></span> ",
				eventCal.Color, dayStr))
		} else if isWeekend {
			output.WriteString(fmt.Sprintf("<span foreground=\"%s\">%s</span> ", weekends, dayStr))
		} else {
			output.WriteString(fmt.Sprintf("<span foreground=\"%s\">%s</span> ", text, dayStr))
		}

		currWeekDay++
	}
	output.WriteString("\n")

	// Output the pre-built event lines
	output.WriteString("\n")
	for _, line := range eventLines {
		output.WriteString(line + "\n")
	}

	fmt.Print(output.String())
}

// getEventCalendarOnDayForTooltip returns the calendar for an event on the given day
func getEventCalendarOnDayForTooltip(day int, month time.Month, year int, events []Event, holidays []Event, calendars []Calendar, showHolidays bool) *Calendar {
	// Check user events
	for _, event := range events {
		if event.Date.Day() == day &&
			event.Date.Month() == month &&
			event.Date.Year() == year {
			for i := range calendars {
				if calendars[i].Name == event.CalendarName {
					return &calendars[i]
				}
			}
			if len(calendars) > 0 {
				return &calendars[0]
			}
		}
	}

	// Check holidays
	if showHolidays {
		for _, holiday := range holidays {
			if holiday.Date.Day() == day &&
				holiday.Date.Month() == month &&
				holiday.Date.Year() == year {
				for i := range calendars {
					if calendars[i].Name == "holidays" {
						return &calendars[i]
					}
				}
			}
		}
	}

	return nil
}

// getUpcomingEventsForTooltip returns upcoming events starting from today, sorted by date
func getUpcomingEventsForTooltip(today time.Time, events []Event, holidays []Event, showHolidays bool, maxEvents int) []Event {
	var upcomingEvents []Event

	// Check regular events
	for _, event := range events {
		eventDate := time.Date(event.Date.Year(), event.Date.Month(), event.Date.Day(), 0, 0, 0, 0, time.Local)
		if !eventDate.Before(today) {
			upcomingEvents = append(upcomingEvents, event)
		}
	}

	// Check holidays
	if showHolidays {
		for _, holiday := range holidays {
			holidayDate := time.Date(holiday.Date.Year(), holiday.Date.Month(), holiday.Date.Day(), 0, 0, 0, 0, time.Local)
			if !holidayDate.Before(today) {
				upcomingEvents = append(upcomingEvents, holiday)
			}
		}
	}

	// Sort by date, then by time
	sort.Slice(upcomingEvents, func(i, j int) bool {
		if !upcomingEvents[i].Date.Equal(upcomingEvents[j].Date) {
			return upcomingEvents[i].Date.Before(upcomingEvents[j].Date)
		}
		if upcomingEvents[i].AllDay && !upcomingEvents[j].AllDay {
			return true
		}
		if !upcomingEvents[i].AllDay && upcomingEvents[j].AllDay {
			return false
		}
		return upcomingEvents[i].Time < upcomingEvents[j].Time
	})

	// Limit to maxEvents
	if len(upcomingEvents) > maxEvents {
		upcomingEvents = upcomingEvents[:maxEvents]
	}

	return upcomingEvents
}

// loadEventIndicatorDays loads the event_indicator_days setting from config
func loadEventIndicatorDays() int {
	indicatorDays := 0 // default: only show indicator on day of event

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return indicatorDays
	}
	configPath := filepath.Join(homeDir, ".config", "zen-cal", "zen-cal.conf")
	file, err := os.Open(configPath)
	if err != nil {
		return indicatorDays
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		if key == "event_indicator_days" {
			if n, err := strconv.Atoi(val); err == nil && n >= 0 {
				indicatorDays = n
			}
		}
	}

	return indicatorDays
}

// RunWaybarMode outputs JSON for Waybar custom module with text and tooltip
func RunWaybarMode() {
	year, month, day := time.Now().Date()

	// Build the display text (matches format " {:%a %d }")
	dateText := time.Now().Format("Mon 02")

	// Load configuration and events to check if there are events today
	_, _, showHolidays, _, _, _ := loadDisplayConfig()
	indicatorDays := loadEventIndicatorDays()
	calendars, vdirEvents := detectVdirsyncerCalendars()
	var events []Event
	if len(calendars) == 0 {
		calendars = loadCalendars()
		events = loadEvents(calendars)
	} else {
		calendars = mergeCalendarConfigs(calendars)
		events = vdirEvents
	}

	var holidays []Event
	if showHolidays {
		holidays = generateUSHolidays(year)
	}

	// Check if there are upcoming events
	todayDate := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	indicatorEndDate := todayDate.AddDate(0, 0, indicatorDays)
	upcomingEvents := getUpcomingEventsForTooltip(todayDate, events, holidays, showHolidays, 3)

	// Set alt field for format-icons - show has-events if there are events within indicator range
	alt := "no-events"
	for _, event := range upcomingEvents {
		eventDate := time.Date(event.Date.Year(), event.Date.Month(), event.Date.Day(), 0, 0, 0, 0, time.Local)
		// Check if event is within the indicator range (today to today + indicatorDays)
		if !eventDate.Before(todayDate) && !eventDate.After(indicatorEndDate) {
			alt = "has-events"
			break
		}
	}

	// Build the tooltip
	tooltip := buildTooltipString(year, month, day)

	// Escape the tooltip for JSON
	tooltip = escapeJSON(tooltip)

	// Output JSON format for Waybar
	fmt.Printf(`{"text": "%s", "alt": "%s", "tooltip": "%s"}`, dateText, alt, tooltip)
}

// buildTooltipString generates the Pango markup tooltip content
func buildTooltipString(year int, month time.Month, day int) string {
	// Load configuration
	_, _, showHolidays, _, _, _ := loadDisplayConfig()
	today, _, headings, text, weekends, _ := getPalette()

	// Load calendars and events (match main app logic)
	calendars, vdirEvents := detectVdirsyncerCalendars()
	var events []Event
	if len(calendars) == 0 {
		calendars = loadCalendars()
		events = loadEvents(calendars)
	} else {
		calendars = mergeCalendarConfigs(calendars)
		events = vdirEvents
	}

	// Load holidays if enabled
	var holidays []Event
	if showHolidays {
		holidays = generateUSHolidays(year)
	}

	// Build calendar output
	// Get upcoming events (next 3) - build these first to calculate max width
	todayDate := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	upcomingEvents := getUpcomingEventsForTooltip(todayDate, events, holidays, showHolidays, 3)

	// Build upcoming events lines and calculate max width
	var eventLines []string
	maxEventWidth := 0

	if len(upcomingEvents) > 0 {
		headerLine := fmt.Sprintf("<span foreground=\"%s\"><b>Upcoming Events</b></span>", headings)
		eventLines = append(eventLines, headerLine)
		if w := pangoVisibleLength(headerLine); w > maxEventWidth {
			maxEventWidth = w
		}

		for _, event := range upcomingEvents {
			calColor := string(text)
			for _, cal := range calendars {
				if cal.Name == event.CalendarName {
					calColor = string(cal.Color)
					break
				}
			}

			timeStr := "All-day"
			if !event.AllDay && event.Time != "" {
				timeStr = event.Time
			}

			fbStr := ""
			if event.FreeBusy == StatusFree {
				fbStr = " <span foreground=\"#a6e3a1\">(Free)</span>"
			}

			prefix := ""
			if event.CalendarName == "holidays" {
				prefix = "🎉 "
			}

			// Date string for upcoming events
			dateStr := event.Date.Format("Jan 02")

			line := fmt.Sprintf("<span foreground=\"%s\">●</span> <span foreground=\"%s\">%s %s</span> %s%s%s",
				calColor, headings, dateStr, timeStr, prefix, event.Title, fbStr)
			eventLines = append(eventLines, line)
			if w := pangoVisibleLength(line); w > maxEventWidth {
				maxEventWidth = w
			}
		}
	} else {
		line := fmt.Sprintf("<span foreground=\"%s\" style=\"italic\">No upcoming events</span>", text)
		eventLines = append(eventLines, line)
		if w := pangoVisibleLength(line); w > maxEventWidth {
			maxEventWidth = w
		}
	}

	// Calculate padding to center the 21-char calendar within the max event width
	calendarWidth := 21
	calendarPadding := ""
	if maxEventWidth > calendarWidth {
		padAmount := (maxEventWidth - calendarWidth) / 2
		calendarPadding = strings.Repeat(" ", padAmount)
	}

	var output strings.Builder

	// Title - centered over the calendar grid
	titleText := fmt.Sprintf("%s %d", month.String(), year)
	titlePadding := (calendarWidth - len(titleText)) / 2
	if titlePadding < 0 {
		titlePadding = 0
	}
	output.WriteString(fmt.Sprintf("<span foreground=\"%s\"><b>%s%s</b></span>\n\n",
		text, calendarPadding+strings.Repeat(" ", titlePadding), titleText))

	// Weekday headers
	headers := []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
	output.WriteString(calendarPadding)
	for i, header := range headers {
		color := headings
		if i == 0 || i == 6 {
			color = weekends
		}
		output.WriteString(fmt.Sprintf("<span foreground=\"%s\"><i>%s</i></span> ", color, header))
	}
	output.WriteString("\n")

	// Get month info
	firstDay := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	firstWeekday := int(firstDay.Weekday())
	lastDay := firstDay.AddDate(0, 1, -1).Day()

	// Build calendar grid
	currWeekDay := firstWeekday

	// Leading spaces (with calendar padding for first row)
	output.WriteString(calendarPadding)
	for i := 0; i < currWeekDay; i++ {
		output.WriteString("   ")
	}

	// Days
	for d := 1; d <= lastDay; d++ {
		if currWeekDay%7 == 0 && d != 1 {
			output.WriteString("\n" + calendarPadding)
		}

		dayStr := fmt.Sprintf("%2d", d)
		isToday := d == day
		isWeekend := currWeekDay%7 == 0 || currWeekDay%7 == 6
		eventCal := getEventCalendarOnDayForTooltip(d, month, year, events, holidays, calendars, showHolidays)

		if isToday {
			output.WriteString(fmt.Sprintf("<span background=\"%s\" foreground=\"%s\"><b>%s</b></span> ",
				today, "#1e1e2e", dayStr))
		} else if eventCal != nil {
			output.WriteString(fmt.Sprintf("<span foreground=\"%s\"><u>%s</u></span> ",
				eventCal.Color, dayStr))
		} else if isWeekend {
			output.WriteString(fmt.Sprintf("<span foreground=\"%s\">%s</span> ", weekends, dayStr))
		} else {
			output.WriteString(fmt.Sprintf("<span foreground=\"%s\">%s</span> ", text, dayStr))
		}

		currWeekDay++
	}
	output.WriteString("\n")

	// Output the pre-built event lines
	output.WriteString("\n")
	for _, line := range eventLines {
		output.WriteString(line + "\n")
	}

	return output.String()
}

// escapeJSON escapes special characters for JSON string
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// CheckTooltipFlag checks if --tooltip or --waybar flag was passed
func CheckTooltipFlag() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--tooltip" || arg == "-t" {
			RunTooltipMode()
			return true
		}
		if arg == "--waybar" || arg == "-w" {
			RunWaybarMode()
			return true
		}
	}
	return false
}