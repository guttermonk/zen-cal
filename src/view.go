package main

import (
	"fmt"
	"os"
	"strings"

	lipgloss "github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// VIEW
func (c calendarPage) View() string {
	return buildCal(c)
}

func buildCal(c calendarPage) string {
	styles := c.styles

	firstWeekDay, week, lastDay := getMonthInfo(c.selMonth, c.selYear)

	var cal strings.Builder

	// Header row: week numbers (optional) + weekdays
	if c.showWeekNumbers {
		headers := [8]string{"#", "Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
		for _, header := range headers {
			cal.WriteString(styles.headerStyle.Render(header))
		}
	} else {
		headers := [7]string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
		for _, header := range headers {
			cal.WriteString(styles.headerStyle.Render(header))
		}
	}
	cal.WriteByte('\n')

	// Start with first week number (if enabled)
	if c.showWeekNumbers {
		cal.WriteString(styles.weekNumStyle.Render(fmt.Sprintf("%d", week)))
	}

	currWeekDay := int(firstWeekDay)
	// Fill empty days before the first of the month
	for i := 0; i < currWeekDay; i++ {
		cal.WriteString(styles.weekdayStyle.Render("    ")) // 4-char width placeholder
	}

	// Render days of the month
	for d := 1; d <= lastDay; d++ {
		if currWeekDay%7 == 0 && d != 1 {
			cal.WriteByte('\n')
			week++
			if c.showWeekNumbers {
				cal.WriteString(styles.weekNumStyle.Render(fmt.Sprintf("%d", week)))
			}
		}

		dayStr := fmt.Sprintf("%d", d)
		var style lipgloss.Style

		isToday := d == c.currDay && c.currYear == c.selYear && c.selMonth == c.currMonth
		isSelected := d == c.selDay
		isWeekend := currWeekDay%7 == 0 || currWeekDay%7 == 6
		eventCal := c.getEventCalendarOnDay(d, c.selMonth, c.selYear)

		switch {
		case isToday && isSelected:
			// Both today and selected - use today style with extra emphasis
			style = styles.todayStyle
		case isSelected:
			// Selected day (not today)
			style = styles.selectedStyle
		case isToday:
			style = styles.todayStyle
		case eventCal != nil:
			// Day has event(s) - use calendar's color
			style = lipgloss.NewStyle().
				Width(4).
				Align(lipgloss.Center).
				Foreground(eventCal.Color).
				Underline(true)
		case isWeekend:
			style = styles.weekendStyle
		default:
			style = styles.weekdayStyle
		}

		cal.WriteString(style.Render(dayStr))
		currWeekDay++
	}

	// Calculate calendar width for centering title
	calStr := cal.String()
	calWidth := lipgloss.Width(calStr)

	// Title - show moving indicator if in move mode
	var titleText string
	if c.mode == ModeMoveEvent && c.movingEventIndex >= 0 && c.movingEventIndex < len(c.events) {
		eventName := c.events[c.movingEventIndex].Title
		if len(eventName) > 15 {
			eventName = eventName[:12] + "..."
		}
		titleText = fmt.Sprintf("Moving: %s", eventName)
	} else {
		titleText = fmt.Sprintf("%s %d", c.selMonth, c.selYear)
	}

	title := styles.titleStyle.
		Width(calWidth).
		Align(lipgloss.Center).
		Render(titleText)

	// Build calendar block (without legend/footer - they go at the bottom)
	calendarBlock := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		calStr,
	)

	// Calculate available height for events list
	// Calendar height is what we want the events to match
	calHeight := lipgloss.Height(calendarBlock)

	// Fixed dimensions for stable layout
	eventsWidth := 32 // Fixed content width for events
	calBlockWidth := lipgloss.Width(calendarBlock)
	fullWidth := calBlockWidth + eventsWidth + 5 // calendar + events + border/padding

	// Fixed events height that always includes space for scroll indicators
	// This prevents vertical jumping when indicators appear/disappear
	// Use minimum height of 15 to show 3 events with both scroll indicators
	// Content: header (1) + scroll up (1) + 3 events (3+1 + 3+1 + 3) + scroll down (1) + buffer (1) = 15
	eventsHeight := calHeight
	if eventsHeight < 15 {
		eventsHeight = 15
	}

	// Build upcoming events list with scroll support
	eventsBlock := buildEventsList(c, eventsHeight, eventsWidth)
	
	// Apply fixed dimensions to prevent legend jumping
	eventsBlock = lipgloss.NewStyle().
		Width(eventsWidth).
		MaxWidth(eventsWidth).
		Height(eventsHeight).
		Render(eventsBlock)

	// Join calendar and events horizontally with fixed dimensions
	mainContent := lipgloss.NewStyle().
		Width(fullWidth).
		Height(eventsHeight).
		Render(lipgloss.JoinHorizontal(
			lipgloss.Top,
			calendarBlock,
			eventsBlock,
		))

	// Build legend (if enabled) - spans full width
	legend := ""
	if c.showLegend {
		legend = buildLegend(c, fullWidth)
	}

	// Footer changes based on mode - spans full width
	footerText := renderFooter(c)
	footer := lipgloss.NewStyle().
		Width(fullWidth).
		Align(lipgloss.Center).
		Render(footerText)

	// Build full layout with legend and footer at the bottom
	var fullCal string
	if c.showLegend && legend != "" {
		fullCal = lipgloss.NewStyle().
			Width(fullWidth).
			Render(lipgloss.JoinVertical(
				lipgloss.Left,
				mainContent,
				"",
				legend,
				footer,
			))
	} else {
		fullCal = lipgloss.NewStyle().
			Width(fullWidth).
			Render(lipgloss.JoinVertical(
				lipgloss.Left,
				mainContent,
				"",
				footer,
			))
	}

	// Add popup overlay if in popup mode
	if c.mode == ModeAddPopup || c.mode == ModeEditPopup || c.mode == ModeDeletePopup {
		fullCal = renderWithPopup(c, fullCal)
	}

	fd := int(os.Stdout.Fd())
	// Center horizontally and vertically in terminal
	termWidth, termHeight, err := term.GetSize(fd)
	if err != nil {
		return fullCal
	} else {
		return lipgloss.Place(
			termWidth-1,
			termHeight-1,
			lipgloss.Center,
			lipgloss.Center,
			fullCal,
		)
	}
}

func buildLegend(c calendarPage, width int) string {
	labelStyle := lipgloss.NewStyle().
		Faint(true)

	// Build legend items for each calendar
	var items []string

	// Calendar indicators
	for _, cal := range c.calendars {
		calDot := lipgloss.NewStyle().
			Foreground(cal.Color).
			Render("●")
		items = append(items, fmt.Sprintf("%s %s", calDot, labelStyle.Render(cal.DisplayName)))
	}

	legend := strings.Join(items, "  ")

	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(legend)
}

func renderFooter(c calendarPage) string {
	_, _, _, text, _, keys := getPalette()

	keyStyle := lipgloss.NewStyle().Foreground(keys).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(text)

	// Helper to format "key action" pairs
	formatPair := func(key, action string) string {
		return keyStyle.Render(key) + textStyle.Render(" "+action)
	}

	var parts []string

	switch c.mode {
	case ModeNormal:
		parts = []string{
			formatPair("⇧←→", "month"),
			formatPair("⇧↑↓", "year"),
			formatPair("r", "reset"),
			formatPair("↵", "events"),
		}
	case ModeEventList:
		parts = []string{
			formatPair("↑↓", "select"),
			formatPair("↵", "edit"),
			formatPair("a", "add"),
			formatPair("d", "del"),
			formatPair("m", "move"),
			formatPair("/", "search"),
		}
	case ModeAddPopup, ModeEditPopup:
		parts = []string{
			formatPair("tab", "next"),
			formatPair("↵", "confirm"),
			formatPair("esc", "cancel"),
		}
	case ModeDeletePopup:
		parts = []string{
			formatPair("y", "confirm"),
			formatPair("n", "cancel"),
			formatPair("esc", "cancel"),
		}
	case ModeMoveEvent:
		parts = []string{
			formatPair("⇧←→", "month"),
			formatPair("⇧↑↓", "year"),
			formatPair("↵", "confirm"),
			formatPair("esc", "cancel"),
		}
	case ModeSearch:
		if c.searchNavigating {
			parts = []string{
				formatPair("↑↓", "select"),
				formatPair("→", "go to"),
				formatPair("←", "back"),
			}
		} else {
			parts = []string{
				formatPair("type", "search"),
				formatPair("↵", "navigate"),
				formatPair("esc", "cancel"),
			}
		}
	default:
		return ""
	}

	return strings.Join(parts, "  ")
}

func buildEventsList(c calendarPage, maxHeight int, maxWidth int) string {
	upcoming := c.GetUpcomingEvents()
	upcomingIndices := c.GetUpcomingEventsWithIndices()

	_, _, headings, text, _, _ := getPalette()

	// Title style for events section
	eventTitleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(headings).
		MarginBottom(1)

	// Event item styles
	eventDateStyle := lipgloss.NewStyle().
		Foreground(headings).
		Italic(true)

	eventNameStyle := lipgloss.NewStyle().
		Foreground(text).
		Bold(true)

	eventDescStyle := lipgloss.NewStyle().
		Foreground(text).
		Faint(true)

	// Selection indicator for selected event
	selectionIndicator := lipgloss.NewStyle().
		Foreground(headings).
		Bold(true).
		Render("▶ ")
	noSelectionPadding := "  " // Same width as indicator

	// Scroll indicator style
	scrollIndicatorStyle := lipgloss.NewStyle().
		Foreground(headings).
		Faint(true)

	var events strings.Builder

	// Title changes based on mode
	titleText := "Upcoming Events"
	if c.mode == ModeMoveEvent {
		titleText = "Select New Date"
	} else if c.mode == ModeSearch {
		titleText = "🔍 Search Events"
	}
	events.WriteString(eventTitleStyle.Render(titleText))
	events.WriteByte('\n')

	// Show search UI if in search mode
	if c.mode == ModeSearch {
		searchInputStyle := lipgloss.NewStyle().
			Foreground(text).
			Background(lipgloss.Color("#45475a")).
			Padding(0, 1)

		searchQuery := c.searchQuery
		if searchQuery == "" {
			searchQuery = "Type to search..."
		}
		
		// Show cursor only in typing mode, not navigation mode
		if c.searchNavigating {
			events.WriteString(searchInputStyle.Render(searchQuery))
		} else {
			events.WriteString(searchInputStyle.Render(searchQuery + "▎"))
		}
		events.WriteByte('\n')

		if len(c.searchResults) == 0 && c.searchQuery != "" {
			events.WriteString(eventDescStyle.Render("No results found"))
		} else if len(c.searchResults) > 0 {
			// Calculate visible window (3 results at a time, like events list)
			visibleResults := 3
			startIdx := 0
			endIdx := len(c.searchResults)

			if len(c.searchResults) > visibleResults {
				halfVisible := visibleResults / 2
				startIdx = c.searchIndex - halfVisible
				if startIdx < 0 {
					startIdx = 0
				}
				endIdx = startIdx + visibleResults
				if endIdx > len(c.searchResults) {
					endIdx = len(c.searchResults)
					startIdx = endIdx - visibleResults
					if startIdx < 0 {
						startIdx = 0
					}
				}
			}

			needsScrolling := len(c.searchResults) > visibleResults

			// Show scroll up indicator if needed
			if needsScrolling && startIdx > 0 {
				events.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ↑ %d more above", startIdx)))
				events.WriteByte('\n')
			}

			// Show search results
			for i := startIdx; i < endIdx; i++ {
				idx := c.searchResults[i]
				if idx < 0 || idx >= len(c.events) {
					continue
				}
				event := c.events[idx]

				var resultBlock strings.Builder
				dateStr := event.Date.Format("Jan 02, 2006")
				cal := c.getCalendarByName(event.CalendarName)
				if cal != nil {
					calDot := lipgloss.NewStyle().Foreground(cal.Color).Render("●")
					resultBlock.WriteString(calDot + " ")
				}
				resultBlock.WriteString(eventDateStyle.Render(dateStr))
				resultBlock.WriteByte('\n')
				resultBlock.WriteString(eventNameStyle.Render(event.Title))

				resultContent := resultBlock.String()
				// Add selection indicator
				lines := strings.Split(resultContent, "\n")
				var prefixedLines []string
				for j, line := range lines {
					if i == c.searchIndex {
						if j == 0 {
							prefixedLines = append(prefixedLines, selectionIndicator+line)
						} else {
							prefixedLines = append(prefixedLines, noSelectionPadding+line)
						}
					} else {
						prefixedLines = append(prefixedLines, noSelectionPadding+line)
					}
				}
				events.WriteString(strings.Join(prefixedLines, "\n"))

				if i < endIdx-1 {
					events.WriteByte('\n')
					events.WriteByte('\n')
				}
			}

			// Show scroll down indicator if needed
			if needsScrolling && endIdx < len(c.searchResults) {
				events.WriteByte('\n')
				events.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ↓ %d more below", len(c.searchResults)-endIdx)))
			}
		}

		// Apply fixed height first, then wrap in styled container
		fixedContent := lipgloss.NewStyle().
			Height(maxHeight).
			Render(events.String())
		return c.eventListStyle.Render(fixedContent)
	}

	if len(upcoming) == 0 {
		noEventsText := "No upcoming events"
		if c.mode == ModeEventList {
			noEventsText = "No events (press 'a' to add)"
		}
		events.WriteString(lipgloss.NewStyle().
			Foreground(text).
			Faint(true).
			Render(noEventsText))
	} else {
		// Calculate how many lines each event takes
		// and determine visible range based on selected index
		linesPerEvent := 3 // date + title + time/status (spacing handled separately)
		headerLines := 2   // title + newline
		availableLines := maxHeight - headerLines - 2 // reserve space for scroll indicators
		visibleEvents := availableLines / linesPerEvent
		if visibleEvents < 1 {
			visibleEvents = 1
		}

		// Calculate scroll window centered on selected event
		startIdx := 0
		endIdx := len(upcoming)

		if len(upcoming) > visibleEvents {
			// Center the selected event in the visible window
			halfVisible := visibleEvents / 2
			startIdx = c.selectedEventIndex - halfVisible
			if startIdx < 0 {
				startIdx = 0
			}
			endIdx = startIdx + visibleEvents
			if endIdx > len(upcoming) {
				endIdx = len(upcoming)
				startIdx = endIdx - visibleEvents
				if startIdx < 0 {
					startIdx = 0
				}
			}
		}

		// Only show scroll indicators when scrolling is possible
		needsScrolling := len(upcoming) > visibleEvents

		// Show scroll up indicator if needed
		if needsScrolling && startIdx > 0 {
			events.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ↑ %d more above", startIdx)))
			events.WriteByte('\n')
		}

		for i := startIdx; i < endIdx; i++ {
			event := upcoming[i]
			var eventBlock strings.Builder

			// Check if this is a holiday
			isHoliday := event.CalendarName == "holidays"

			// Calculate content width (accounting for selection indicator and safety margin)
			contentWidth := maxWidth - 6 // Account for "▶ " prefix and extra safety margin

			// Helper to truncate strings by rune count (handles unicode better)
			truncate := func(s string, maxLen int) string {
				runes := []rune(s)
				if len(runes) <= maxLen {
					return s
				}
				if maxLen <= 3 {
					if maxLen > 0 && maxLen <= len(runes) {
						return string(runes[:maxLen])
					}
					return ""
				}
				return string(runes[:maxLen-3]) + "..."
			}

			// Format: date with calendar color indicator
			dateStr := event.Date.Format("Jan 02, 2006")
			cal := c.getCalendarByName(event.CalendarName)
			if cal != nil {
				calDot := lipgloss.NewStyle().Foreground(cal.Color).Render("●")
				eventBlock.WriteString(calDot + " ")
			}
			eventBlock.WriteString(eventDateStyle.Render(dateStr))
			eventBlock.WriteByte('\n')

			// Format: title (with holiday indicator) - truncate to fit
			titleStr := event.Title
			if isHoliday {
				titleStr = "🎉 " + titleStr
			}
			titleStr = truncate(titleStr, contentWidth)
			eventBlock.WriteString(eventNameStyle.Render(titleStr))
			eventBlock.WriteByte('\n')

			// Format: time (or All-day)
			timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a6adc8"))
			var timeStr string
			if event.AllDay {
				timeStr = "All-day"
			} else if event.Time != "" {
				timeStr = event.Time
			} else {
				timeStr = "All-day"
			}

			// Format: free/busy status
			var freeBusyStyle lipgloss.Style
			var freeBusyStr string
			if event.FreeBusy == StatusFree {
				freeBusyStr = "Free"
				freeBusyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))
			} else {
				freeBusyStr = "Busy"
				freeBusyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
			}

			eventBlock.WriteString(timeStyle.Render(timeStr))
			eventBlock.WriteString("  ")
			eventBlock.WriteString(freeBusyStyle.Render(freeBusyStr))

			// Apply selection indicator if in event list mode
			eventContent := eventBlock.String()
			if c.mode == ModeEventList {
				// Add selection indicator or padding to each line
				lines := strings.Split(eventContent, "\n")
				var prefixedLines []string
				for j, line := range lines {
					if i == c.selectedEventIndex && i < len(upcomingIndices) {
						if j == 0 {
							prefixedLines = append(prefixedLines, selectionIndicator+line)
						} else {
							prefixedLines = append(prefixedLines, noSelectionPadding+line)
						}
					} else {
						prefixedLines = append(prefixedLines, noSelectionPadding+line)
					}
				}
				eventContent = strings.Join(prefixedLines, "\n")
			}

			events.WriteString(eventContent)

			// Add spacing between events (except last visible)
			if i < endIdx-1 {
				events.WriteByte('\n')
				events.WriteByte('\n')
			}
		}

		// Show scroll down indicator if needed
		if needsScrolling && endIdx < len(upcoming) {
			events.WriteByte('\n')
			events.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ↓ %d more below", len(upcoming)-endIdx)))
		}
	}

	// Apply fixed height first, then wrap in styled container
	fixedContent := lipgloss.NewStyle().
		Height(maxHeight).
		Render(events.String())
	return c.eventListStyle.Render(fixedContent)
}

// applyFixedHeight applies a fixed height to content to keep layout stable
func applyFixedHeight(content string, maxHeight int) string {
	return lipgloss.NewStyle().
		Height(maxHeight).
		Render(content)
}

func renderWithPopup(c calendarPage, background string) string {
	var popup string

	switch c.mode {
	case ModeAddPopup:
		popup = renderAddEditPopup(c, "Add New Event")
	case ModeEditPopup:
		popup = renderAddEditPopup(c, "Edit Event")
	case ModeDeletePopup:
		popup = renderDeletePopup(c)
	}

	// Get dimensions
	bgWidth := lipgloss.Width(background)
	bgHeight := lipgloss.Height(background)
	popupWidth := lipgloss.Width(popup)
	popupHeight := lipgloss.Height(popup)

	// Create a container that's the same size as the background
	// and place the popup in the center
	centeredPopup := lipgloss.Place(
		bgWidth,
		bgHeight,
		lipgloss.Center,
		lipgloss.Center,
		popup,
	)

	// Since we can't easily overlay, just return the centered popup
	// The popup has its own border and styling, so it looks fine standalone
	_ = centeredPopup
	_ = popupWidth
	_ = popupHeight

	// For a cleaner look, just show the popup centered in the available space
	return lipgloss.Place(
		bgWidth,
		bgHeight,
		lipgloss.Center,
		lipgloss.Center,
		popup,
	)
}

func renderAddEditPopup(c calendarPage, title string) string {
	_, _, headings, text, _, _ := getPalette()
	today, todayText, _, _, _, _ := getPalette()

	// Popup container style
	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(headings).
		Padding(1, 2)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(headings)

	labelStyle := lipgloss.NewStyle().
		Foreground(text).
		Width(13).
		Align(lipgloss.Right).
		PaddingRight(1)

	inputStyle := lipgloss.NewStyle().
		Foreground(text)

	focusedInputStyle := lipgloss.NewStyle().
		Foreground(todayText).
		Background(today)

	dimStyle := lipgloss.NewStyle().
		Foreground(text).
		Faint(true)

	toggleStyle := lipgloss.NewStyle().
		Foreground(text).
		Faint(true).
		PaddingLeft(1).
		PaddingRight(1)

	activeToggleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true).
		PaddingLeft(1).
		PaddingRight(1)

	focusedToggleStyle := lipgloss.NewStyle().
		Foreground(todayText).
		Background(today).
		PaddingLeft(1).
		PaddingRight(1)

	buttonStyle := lipgloss.NewStyle().
		Foreground(text).
		PaddingLeft(2).
		PaddingRight(2)

	focusedButtonStyle := lipgloss.NewStyle().
		Background(today).
		Foreground(todayText).
		PaddingLeft(2).
		PaddingRight(2)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#f38ba8")).
		Italic(true)

	var rows []string

	// Title
	rows = append(rows, titleStyle.Render(title))
	rows = append(rows, "")

	// Date field
	dateInput := c.formState.DateInput
	if dateInput == "" {
		dateInput = "YYYY-MM-DD"
	}
	dateStyle := inputStyle
	if c.formState.FocusedField == FieldDate {
		dateStyle = focusedInputStyle
		// Insert cursor at position
		pos := c.formState.CursorPos
		if pos > len(dateInput) {
			pos = len(dateInput)
		}
		dateInput = dateInput[:pos] + "▎" + dateInput[pos:]
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center,
		labelStyle.Render("Date:"),
		dateStyle.Render(dateInput),
	))

	// Time field
	timeInput := c.formState.TimeInput
	if c.formState.AllDayInput {
		timeInput = "N/A"
		timeStyle := dimStyle
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center,
			labelStyle.Render("Time:"),
			timeStyle.Render(timeInput),
		))
	} else {
		if timeInput == "" {
			timeInput = "HH:MM"
		}
		timeStyle := inputStyle
		if c.formState.FocusedField == FieldTime {
			timeStyle = focusedInputStyle
			// Insert cursor at position
			pos := c.formState.CursorPos
			if pos > len(timeInput) {
				pos = len(timeInput)
			}
			timeInput = timeInput[:pos] + "▎" + timeInput[pos:]
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center,
			labelStyle.Render("Time:"),
			timeStyle.Render(timeInput),
		))

		// End Time field (only shown when not all-day)
		endTimeInput := c.formState.EndTimeInput
		if endTimeInput == "" {
			endTimeInput = "HH:MM"
		}
		endTimeStyle := inputStyle
		if c.formState.FocusedField == FieldEndTime {
			endTimeStyle = focusedInputStyle
			pos := c.formState.CursorPos
			if pos > len(endTimeInput) {
				pos = len(endTimeInput)
			}
			endTimeInput = endTimeInput[:pos] + "▎" + endTimeInput[pos:]
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center,
			labelStyle.Render("End Time:"),
			endTimeStyle.Render(endTimeInput),
		))
	}

	// All-day toggle
	var yesStyle, noStyle lipgloss.Style
	if c.formState.FocusedField == FieldAllDay {
		if c.formState.AllDayInput {
			yesStyle = focusedToggleStyle
			noStyle = toggleStyle
		} else {
			yesStyle = toggleStyle
			noStyle = focusedToggleStyle
		}
	} else {
		if c.formState.AllDayInput {
			yesStyle = activeToggleStyle
			noStyle = toggleStyle
		} else {
			yesStyle = toggleStyle
			noStyle = activeToggleStyle
		}
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center,
		labelStyle.Render("All-day:"),
		yesStyle.Render("Yes"),
		" ",
		noStyle.Render("No"),
	))

	// Title field
	titleInput := c.formState.TitleInput
	if titleInput == "" {
		titleInput = "Event title"
	}
	titleInputStyle := inputStyle
	if c.formState.FocusedField == FieldTitle {
		titleInputStyle = focusedInputStyle
		// Insert cursor at position
		pos := c.formState.CursorPos
		if pos > len(titleInput) {
			pos = len(titleInput)
		}
		titleInput = titleInput[:pos] + "▎" + titleInput[pos:]
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center,
		labelStyle.Render("Title:"),
		titleInputStyle.Render(titleInput),
	))

	// Description field
	descInput := c.formState.DescriptionInput
	if descInput == "" {
		descInput = "Optional"
	}
	descStyle := inputStyle
	if c.formState.FocusedField == FieldDescription {
		descStyle = focusedInputStyle
		// Insert cursor at position
		pos := c.formState.CursorPos
		if pos > len(descInput) {
			pos = len(descInput)
		}
		descInput = descInput[:pos] + "▎" + descInput[pos:]
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center,
		labelStyle.Render("Description:"),
		descStyle.Render(descInput),
	))

	// Location field
	locInput := c.formState.LocationInput
	if locInput == "" {
		locInput = "Optional"
	}
	locStyle := inputStyle
	if c.formState.FocusedField == FieldLocation {
		locStyle = focusedInputStyle
		// Insert cursor at position
		pos := c.formState.CursorPos
		if pos > len(locInput) {
			pos = len(locInput)
		}
		locInput = locInput[:pos] + "▎" + locInput[pos:]
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center,
		labelStyle.Render("Location:"),
		locStyle.Render(locInput),
	))

	// Calendar field - dropdown style
	calInput := c.formState.CalendarInput
	if calInput == "" && len(c.calendars) > 0 {
		calInput = c.calendars[0].DisplayName
	}
	// Find display name for current calendar
	for _, cal := range c.calendars {
		if cal.Name == c.formState.CalendarInput {
			calInput = cal.DisplayName
			break
		}
	}
	calStyle := inputStyle
	if c.formState.FocusedField == FieldCalendar {
		calStyle = focusedInputStyle
		calInput = "◀ " + calInput + " ▶"
	} else {
		calInput = "  " + calInput + "  "
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center,
		labelStyle.Render("Calendar:"),
		calStyle.Render(calInput),
	))

	// Free/Busy toggle
	var busyStyle, freeStyle lipgloss.Style
	if c.formState.FocusedField == FieldFreeBusy {
		if c.formState.FreeBusyInput == StatusBusy {
			busyStyle = focusedToggleStyle
			freeStyle = toggleStyle
		} else {
			busyStyle = toggleStyle
			freeStyle = focusedToggleStyle
		}
	} else {
		if c.formState.FreeBusyInput == StatusBusy {
			busyStyle = activeToggleStyle
			freeStyle = toggleStyle
		} else {
			busyStyle = toggleStyle
			freeStyle = activeToggleStyle
		}
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center,
		labelStyle.Render("Status:"),
		busyStyle.Render("Busy"),
		" ",
		freeStyle.Render("Free"),
	))

	rows = append(rows, "")

	// Error message if any
	if c.formState.ErrorMsg != "" {
		rows = append(rows, errorStyle.Render("⚠ "+c.formState.ErrorMsg))
		rows = append(rows, "")
	}

	// Buttons
	confirmBtn := buttonStyle
	cancelBtn := buttonStyle
	if c.formState.FocusedField == FieldConfirm {
		confirmBtn = focusedButtonStyle
	}
	if c.formState.FocusedField == FieldCancel {
		cancelBtn = focusedButtonStyle
	}

	buttons := lipgloss.JoinHorizontal(
		lipgloss.Center,
		confirmBtn.Render("Save"),
		"  ",
		cancelBtn.Render("Cancel"),
	)
	rows = append(rows, lipgloss.NewStyle().Width(40).Align(lipgloss.Center).Render(buttons))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return popupStyle.Render(content)
}

func renderDeletePopup(c calendarPage) string {
	_, _, headings, text, _, _ := getPalette()
	today, todayText, _, _, _, _ := getPalette()

	// Popup container style
	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#f38ba8")).
		Padding(1, 2).
		Width(44)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#f38ba8")).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(text)

	eventNameStyle := lipgloss.NewStyle().
		Foreground(headings).
		Bold(true)

	eventDateStyle := lipgloss.NewStyle().
		Foreground(headings).
		Italic(true)

	buttonStyle := lipgloss.NewStyle().
		Foreground(text).
		Border(lipgloss.NormalBorder()).
		BorderForeground(text).
		Padding(0, 2)

	dangerButtonStyle := buttonStyle.
		Background(lipgloss.Color("#f38ba8")).
		Foreground(todayText).
		BorderForeground(lipgloss.Color("#f38ba8"))

	focusedButtonStyle := buttonStyle.
		Background(today).
		Foreground(todayText).
		BorderForeground(today)

	var content strings.Builder

	// Title
	content.WriteString(titleStyle.Render("⚠ Delete Event"))
	content.WriteByte('\n')
	content.WriteByte('\n')

	// Show event being deleted
	if c.editingEventIndex >= 0 && c.editingEventIndex < len(c.events) {
		event := c.events[c.editingEventIndex]
		content.WriteString(textStyle.Render("Are you sure you want to delete:"))
		content.WriteByte('\n')
		content.WriteByte('\n')
		content.WriteString(eventNameStyle.Render("  " + event.Title))
		content.WriteByte('\n')
		content.WriteString(eventDateStyle.Render("  " + event.Date.Format("Jan 02, 2006")))
		if event.Description != "" {
			content.WriteByte('\n')
			content.WriteString(textStyle.Render("  " + event.Description))
		}
		content.WriteByte('\n')
		content.WriteByte('\n')
	}

	// Buttons
	var confirmBtn, cancelBtn lipgloss.Style
	if c.formState.FocusedField == FieldConfirm {
		confirmBtn = dangerButtonStyle
		cancelBtn = buttonStyle
	} else {
		confirmBtn = buttonStyle
		cancelBtn = focusedButtonStyle
	}

	buttons := lipgloss.JoinHorizontal(
		lipgloss.Center,
		confirmBtn.Render("Delete"),
		"  ",
		cancelBtn.Render("Cancel"),
	)
	content.WriteString(lipgloss.NewStyle().Width(40).Align(lipgloss.Center).Render(buttons))

	return popupStyle.Render(content.String())
}