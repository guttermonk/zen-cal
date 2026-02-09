package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// UPDATE
func (c calendarPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		switch c.mode {
		case ModeNormal:
			return c.updateNormalMode(key)
		case ModeEventList:
			return c.updateEventListMode(key)
		case ModeAddPopup:
			return c.updateAddPopupMode(key, msg)
		case ModeEditPopup:
			return c.updateEditPopupMode(key, msg)
		case ModeDeletePopup:
			return c.updateDeletePopupMode(key)
		case ModeMoveEvent:
			return c.updateMoveEventMode(key)
		case ModeSearch:
			return c.updateSearchMode(key)
		}
	}
	return c, nil
}

func (c calendarPage) updateNormalMode(key string) (tea.Model, tea.Cmd) {
	switch {
	case containsKey(c.keybinds.Quit, key):
		// Run blocking sync before quitting to ensure all changes are synced
		if HasPendingSync() {
			RunVdirsyncerSyncBlocking()
		}
		return c, tea.Quit

	// Day/week navigation (regular arrows) - check these BEFORE month/year
	case containsKey(c.keybinds.Right, key): // right/l - next day
		selDate := time.Date(c.selYear, c.selMonth, c.selDay, 0, 0, 0, 0, time.Local)
		selDate = selDate.AddDate(0, 0, 1)
		c.selYear = selDate.Year()
		c.selMonth = selDate.Month()
		c.selDay = selDate.Day()

	case containsKey(c.keybinds.Left, key): // left/h - previous day
		selDate := time.Date(c.selYear, c.selMonth, c.selDay, 0, 0, 0, 0, time.Local)
		selDate = selDate.AddDate(0, 0, -1)
		c.selYear = selDate.Year()
		c.selMonth = selDate.Month()
		c.selDay = selDate.Day()

	case containsKey(c.keybinds.Down, key): // down/j - next week
		selDate := time.Date(c.selYear, c.selMonth, c.selDay, 0, 0, 0, 0, time.Local)
		selDate = selDate.AddDate(0, 0, 7)
		c.selYear = selDate.Year()
		c.selMonth = selDate.Month()
		c.selDay = selDate.Day()

	case containsKey(c.keybinds.Up, key): // up/k - previous week
		selDate := time.Date(c.selYear, c.selMonth, c.selDay, 0, 0, 0, 0, time.Local)
		selDate = selDate.AddDate(0, 0, -7)
		c.selYear = selDate.Year()
		c.selMonth = selDate.Month()
		c.selDay = selDate.Day()

	// Month/year navigation (shift+arrows) - check these AFTER day/week
	case containsKey(c.keybinds.NextYear, key): // shift+down - increase year
		if c.selYear < 9999 {
			c.selYear++
			maxDays := getDaysInMonth(c.selMonth, c.selYear)
			if c.selDay > maxDays {
				c.selDay = maxDays
			}
		}

	case containsKey(c.keybinds.PrevYear, key): // shift+up - decrease year
		if c.selYear > 1 {
			c.selYear--
			maxDays := getDaysInMonth(c.selMonth, c.selYear)
			if c.selDay > maxDays {
				c.selDay = maxDays
			}
		}

	case containsKey(c.keybinds.NextMonth, key): // shift+right - increase month
		if c.selMonth == time.December {
			c.selMonth = time.January
			c.selYear++
		} else {
			c.selMonth++
		}
		maxDays := getDaysInMonth(c.selMonth, c.selYear)
		if c.selDay > maxDays {
			c.selDay = maxDays
		}

	case containsKey(c.keybinds.PrevMonth, key): // shift+left - decrease month
		if c.selMonth == time.January {
			c.selMonth = time.December
			c.selYear--
		} else {
			c.selMonth--
		}
		maxDays := getDaysInMonth(c.selMonth, c.selYear)
		if c.selDay > maxDays {
			c.selDay = maxDays
		}

	case containsKey(c.keybinds.Reset, key): // get new curr time and reset today
		year, month, day := time.Now().Date()
		c.currDay = day
		c.currMonth = month
		c.currYear = year
		c.selMonth = month
		c.selYear = year
		c.selDay = day

	case key == "enter": // Enter event list mode
		c.mode = ModeEventList
		c.selectedEventIndex = 0
	}

	return c, nil
}

func (c calendarPage) updateEventListMode(key string) (tea.Model, tea.Cmd) {
	upcomingIndices := c.GetUpcomingEventsWithIndices()

	switch {
	case key == "esc", key == "q", containsKey(c.keybinds.Left, key):
		c.mode = ModeNormal
		c.selectedEventIndex = 0

	case containsKey(c.keybinds.Reset, key):
		year, month, day := time.Now().Date()
		c.currDay = day
		c.currMonth = month
		c.currYear = year
		c.selMonth = month
		c.selYear = year
		c.selDay = day
		c.mode = ModeNormal
		c.selectedEventIndex = 0

	case containsKey(c.keybinds.Up, key):
		if c.selectedEventIndex > 0 {
			c.selectedEventIndex--
		}

	case containsKey(c.keybinds.Down, key):
		if c.selectedEventIndex < len(upcomingIndices)-1 {
			c.selectedEventIndex++
		}

	case key == "a", key == "A": // Add new event
		c.mode = ModeAddPopup
		c.initAddForm()

	case key == "enter", containsKey(c.keybinds.Right, key): // Edit selected event
		if len(upcomingIndices) > 0 && c.selectedEventIndex < len(upcomingIndices) {
			realIndex := upcomingIndices[c.selectedEventIndex]
			// Don't allow editing holidays (realIndex is -1 for holidays)
			if realIndex >= 0 && realIndex < len(c.events) {
				c.mode = ModeEditPopup
				c.initEditForm(realIndex)
			}
		}

	case key == "d", key == "D": // Delete selected event
		if len(upcomingIndices) > 0 && c.selectedEventIndex < len(upcomingIndices) {
			realIndex := upcomingIndices[c.selectedEventIndex]
			// Don't allow deleting holidays (realIndex is -1 for holidays)
			if realIndex >= 0 && realIndex < len(c.events) {
				c.mode = ModeDeletePopup
				c.initDeleteConfirm(realIndex)
			}
		}

	case key == "m", key == "M": // Move selected event to a different day
		if len(upcomingIndices) > 0 && c.selectedEventIndex < len(upcomingIndices) {
			realIndex := upcomingIndices[c.selectedEventIndex]
			// Don't allow moving holidays (realIndex is -1 for holidays)
			if realIndex >= 0 && realIndex < len(c.events) {
				c.movingEventIndex = realIndex
				c.mode = ModeMoveEvent
			}
		}

	case key == "/": // Search events
		c.mode = ModeSearch
		c.searchQuery = ""
		c.searchResults = nil
		c.searchIndex = 0
	}

	return c, nil
}

func (c calendarPage) updateSearchMode(key string) (tea.Model, tea.Cmd) {
	// Two-stage search: typing mode first, then navigation mode after Enter
	
	if c.searchNavigating {
		// Navigation mode - use configured keybinds
		switch {
		case key == "esc", key == "q", containsKey(c.keybinds.Left, key):
			// Exit search mode
			c.mode = ModeEventList
			c.searchQuery = ""
			c.searchResults = nil
			c.searchIndex = 0
			c.searchNavigating = false

		case key == "enter", containsKey(c.keybinds.Right, key):
			// Select result and go to event
			if len(c.searchResults) > 0 {
				c.navigateToSearchResult()
			}
			c.mode = ModeEventList
			c.searchNavigating = false

		case containsKey(c.keybinds.Up, key):
			// Previous search result
			if len(c.searchResults) > 0 && c.searchIndex > 0 {
				c.searchIndex--
			}

		case containsKey(c.keybinds.Down, key):
			// Next search result
			if len(c.searchResults) > 0 && c.searchIndex < len(c.searchResults)-1 {
				c.searchIndex++
			}
		}
	} else {
		// Typing mode - all keys go to search query
		switch key {
		case "esc":
			c.mode = ModeEventList
			c.searchQuery = ""
			c.searchResults = nil
			c.searchIndex = 0

		case "enter":
			// Switch to navigation mode if there are results
			if len(c.searchResults) > 0 {
				c.searchNavigating = true
			}

		case "backspace":
			if len(c.searchQuery) > 0 {
				c.searchQuery = c.searchQuery[:len(c.searchQuery)-1]
				c.performSearch()
			}

		default:
			// Handle text input for search query
			if len(key) == 1 {
				c.searchQuery += key
				c.performSearch()
			} else if key == "space" {
				c.searchQuery += " "
				c.performSearch()
			}
		}
	}

	return c, nil
}

func (c calendarPage) updateMoveEventMode(key string) (tea.Model, tea.Cmd) {
	switch {
	case key == "esc", key == "q":
		// Cancel move, go back to event list
		c.movingEventIndex = -1
		c.mode = ModeEventList

	case containsKey(c.keybinds.Reset, key):
		year, month, day := time.Now().Date()
		c.currDay = day
		c.currMonth = month
		c.currYear = year
		c.selMonth = month
		c.selYear = year
		c.selDay = day
		c.movingEventIndex = -1
		c.mode = ModeNormal

	// Day/week navigation (regular arrows) - check these BEFORE month/year
	case containsKey(c.keybinds.Right, key): // right/l - next day
		selDate := time.Date(c.selYear, c.selMonth, c.selDay, 0, 0, 0, 0, time.Local)
		selDate = selDate.AddDate(0, 0, 1)
		c.selYear = selDate.Year()
		c.selMonth = selDate.Month()
		c.selDay = selDate.Day()

	case containsKey(c.keybinds.Left, key): // left/h - previous day
		selDate := time.Date(c.selYear, c.selMonth, c.selDay, 0, 0, 0, 0, time.Local)
		selDate = selDate.AddDate(0, 0, -1)
		c.selYear = selDate.Year()
		c.selMonth = selDate.Month()
		c.selDay = selDate.Day()

	case containsKey(c.keybinds.Down, key): // down/j - next week
		selDate := time.Date(c.selYear, c.selMonth, c.selDay, 0, 0, 0, 0, time.Local)
		selDate = selDate.AddDate(0, 0, 7)
		c.selYear = selDate.Year()
		c.selMonth = selDate.Month()
		c.selDay = selDate.Day()

	case containsKey(c.keybinds.Up, key): // up/k - previous week
		selDate := time.Date(c.selYear, c.selMonth, c.selDay, 0, 0, 0, 0, time.Local)
		selDate = selDate.AddDate(0, 0, -7)
		c.selYear = selDate.Year()
		c.selMonth = selDate.Month()
		c.selDay = selDate.Day()

	// Month/year navigation (shift+arrows) - check these AFTER day/week
	case containsKey(c.keybinds.NextYear, key): // shift+down - increase year
		if c.selYear < 9999 {
			c.selYear++
			maxDays := getDaysInMonth(c.selMonth, c.selYear)
			if c.selDay > maxDays {
				c.selDay = maxDays
			}
		}

	case containsKey(c.keybinds.PrevYear, key): // shift+up - decrease year
		if c.selYear > 1 {
			c.selYear--
			maxDays := getDaysInMonth(c.selMonth, c.selYear)
			if c.selDay > maxDays {
				c.selDay = maxDays
			}
		}

	case containsKey(c.keybinds.NextMonth, key): // shift+right - increase month
		if c.selMonth == time.December {
			c.selMonth = time.January
			c.selYear++
		} else {
			c.selMonth++
		}
		maxDays := getDaysInMonth(c.selMonth, c.selYear)
		if c.selDay > maxDays {
			c.selDay = maxDays
		}

	case containsKey(c.keybinds.PrevMonth, key): // shift+left - decrease month
		if c.selMonth == time.January {
			c.selMonth = time.December
			c.selYear--
		} else {
			c.selMonth--
		}
		maxDays := getDaysInMonth(c.selMonth, c.selYear)
		if c.selDay > maxDays {
			c.selDay = maxDays
		}

	case key == "enter": // Confirm move
		c.moveEventToSelectedDay()
		c.mode = ModeEventList
		c.selectedEventIndex = 0
	}

	return c, nil
}

func (c calendarPage) updateAddPopupMode(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		c.mode = ModeEventList
		c.formState = FormState{}

	case "tab", "down":
		// Cycle through fields
		switch c.formState.FocusedField {
		case FieldDate:
			c.formState.FocusedField = FieldTime
			c.formState.CursorPos = len(c.formState.TimeInput)
		case FieldTime:
			c.formState.FocusedField = FieldEndTime
			c.formState.CursorPos = len(c.formState.EndTimeInput)
		case FieldEndTime:
			c.formState.FocusedField = FieldAllDay
			c.formState.CursorPos = 0
		case FieldAllDay:
			c.formState.FocusedField = FieldTitle
			c.formState.CursorPos = len(c.formState.TitleInput)
		case FieldTitle:
			c.formState.FocusedField = FieldDescription
			c.formState.CursorPos = len(c.formState.DescriptionInput)
		case FieldDescription:
			c.formState.FocusedField = FieldLocation
			c.formState.CursorPos = len(c.formState.LocationInput)
		case FieldLocation:
			c.formState.FocusedField = FieldCalendar
			c.formState.CursorPos = 0
		case FieldCalendar:
			c.formState.FocusedField = FieldFreeBusy
			c.formState.CursorPos = 0
		case FieldFreeBusy:
			c.formState.FocusedField = FieldConfirm
			c.formState.CursorPos = 0
		case FieldConfirm:
			c.formState.FocusedField = FieldCancel
			c.formState.CursorPos = 0
		case FieldCancel:
			c.formState.FocusedField = FieldDate
			c.formState.CursorPos = len(c.formState.DateInput)
		}

	case "shift+tab", "up":
		// Cycle backwards
		switch c.formState.FocusedField {
		case FieldDate:
			c.formState.FocusedField = FieldCancel
			c.formState.CursorPos = 0
		case FieldTime:
			c.formState.FocusedField = FieldDate
			c.formState.CursorPos = len(c.formState.DateInput)
		case FieldEndTime:
			c.formState.FocusedField = FieldTime
			c.formState.CursorPos = len(c.formState.TimeInput)
		case FieldAllDay:
			c.formState.FocusedField = FieldEndTime
			c.formState.CursorPos = len(c.formState.EndTimeInput)
		case FieldTitle:
			c.formState.FocusedField = FieldAllDay
			c.formState.CursorPos = 0
		case FieldDescription:
			c.formState.FocusedField = FieldTitle
			c.formState.CursorPos = len(c.formState.TitleInput)
		case FieldLocation:
			c.formState.FocusedField = FieldDescription
			c.formState.CursorPos = len(c.formState.DescriptionInput)
		case FieldCalendar:
			c.formState.FocusedField = FieldLocation
			c.formState.CursorPos = len(c.formState.LocationInput)
		case FieldFreeBusy:
			c.formState.FocusedField = FieldCalendar
			c.formState.CursorPos = 0
		case FieldConfirm:
			c.formState.FocusedField = FieldFreeBusy
			c.formState.CursorPos = 0
		case FieldCancel:
			c.formState.FocusedField = FieldConfirm
			c.formState.CursorPos = 0
		}

	case " ":
		// Toggle for AllDay and FreeBusy fields, or insert space in text fields
		switch c.formState.FocusedField {
		case FieldAllDay:
			c.formState.AllDayInput = !c.formState.AllDayInput
			if c.formState.AllDayInput {
				c.formState.TimeInput = ""
			}
		case FieldFreeBusy:
			if c.formState.FreeBusyInput == StatusBusy {
				c.formState.FreeBusyInput = StatusFree
			} else {
				c.formState.FreeBusyInput = StatusBusy
			}
		case FieldTitle, FieldDescription, FieldLocation:
			// Insert space in text fields
			c.handleTextInput(" ", msg)
		}

	case "left":
		// Move cursor left in text fields, or toggle for special fields
		switch c.formState.FocusedField {
		case FieldAllDay:
			c.formState.AllDayInput = true
			c.formState.TimeInput = ""
		case FieldFreeBusy:
			c.formState.FreeBusyInput = StatusBusy
		case FieldCalendar:
			c.cycleCalendarPrev()
		case FieldDate, FieldTime, FieldEndTime, FieldTitle, FieldDescription, FieldLocation:
			if c.formState.CursorPos > 0 {
				c.formState.CursorPos--
			}
		}

	case "right":
		// Move cursor right in text fields, or toggle for special fields
		switch c.formState.FocusedField {
		case FieldAllDay:
			c.formState.AllDayInput = false
		case FieldFreeBusy:
			c.formState.FreeBusyInput = StatusFree
		case FieldCalendar:
			c.cycleCalendarNext()
		case FieldDate:
			if c.formState.CursorPos < len(c.formState.DateInput) {
				c.formState.CursorPos++
			}
		case FieldTime:
			if c.formState.CursorPos < len(c.formState.TimeInput) {
				c.formState.CursorPos++
			}
		case FieldEndTime:
			if c.formState.CursorPos < len(c.formState.EndTimeInput) {
				c.formState.CursorPos++
			}
		case FieldTitle:
			if c.formState.CursorPos < len(c.formState.TitleInput) {
				c.formState.CursorPos++
			}
		case FieldDescription:
			if c.formState.CursorPos < len(c.formState.DescriptionInput) {
				c.formState.CursorPos++
			}
		case FieldLocation:
			if c.formState.CursorPos < len(c.formState.LocationInput) {
				c.formState.CursorPos++
			}
		}

	case "enter":
		if c.formState.FocusedField == FieldCancel {
			c.mode = ModeEventList
			c.formState = FormState{}
		} else if c.formState.FocusedField == FieldConfirm {
			if c.addEvent() {
				c.mode = ModeEventList
				c.formState = FormState{}
				c.selectedEventIndex = 0
			}
		}

	case "backspace":
		c.handleBackspace()

	default:
		// Handle text input for focused field
		if len(key) == 1 || key == "space" {
			c.handleTextInput(key, msg)
		}
	}

	return c, nil
}

func (c calendarPage) updateEditPopupMode(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		c.mode = ModeEventList
		c.formState = FormState{}
		c.editingEventIndex = -1

	case "tab", "down":
		switch c.formState.FocusedField {
		case FieldDate:
			c.formState.FocusedField = FieldTime
			c.formState.CursorPos = len(c.formState.TimeInput)
		case FieldTime:
			c.formState.FocusedField = FieldEndTime
			c.formState.CursorPos = len(c.formState.EndTimeInput)
		case FieldEndTime:
			c.formState.FocusedField = FieldAllDay
			c.formState.CursorPos = 0
		case FieldAllDay:
			c.formState.FocusedField = FieldTitle
			c.formState.CursorPos = len(c.formState.TitleInput)
		case FieldTitle:
			c.formState.FocusedField = FieldDescription
			c.formState.CursorPos = len(c.formState.DescriptionInput)
		case FieldDescription:
			c.formState.FocusedField = FieldLocation
			c.formState.CursorPos = len(c.formState.LocationInput)
		case FieldLocation:
			c.formState.FocusedField = FieldCalendar
			c.formState.CursorPos = 0
		case FieldCalendar:
			c.formState.FocusedField = FieldFreeBusy
			c.formState.CursorPos = 0
		case FieldFreeBusy:
			c.formState.FocusedField = FieldConfirm
			c.formState.CursorPos = 0
		case FieldConfirm:
			c.formState.FocusedField = FieldCancel
			c.formState.CursorPos = 0
		case FieldCancel:
			c.formState.FocusedField = FieldDate
			c.formState.CursorPos = len(c.formState.DateInput)
		}

	case "shift+tab", "up":
		switch c.formState.FocusedField {
		case FieldDate:
			c.formState.FocusedField = FieldCancel
			c.formState.CursorPos = 0
		case FieldTime:
			c.formState.FocusedField = FieldDate
			c.formState.CursorPos = len(c.formState.DateInput)
		case FieldEndTime:
			c.formState.FocusedField = FieldTime
			c.formState.CursorPos = len(c.formState.TimeInput)
		case FieldAllDay:
			c.formState.FocusedField = FieldEndTime
			c.formState.CursorPos = len(c.formState.EndTimeInput)
		case FieldTitle:
			c.formState.FocusedField = FieldAllDay
			c.formState.CursorPos = 0
		case FieldDescription:
			c.formState.FocusedField = FieldTitle
			c.formState.CursorPos = len(c.formState.TitleInput)
		case FieldLocation:
			c.formState.FocusedField = FieldDescription
			c.formState.CursorPos = len(c.formState.DescriptionInput)
		case FieldCalendar:
			c.formState.FocusedField = FieldLocation
			c.formState.CursorPos = len(c.formState.LocationInput)
		case FieldFreeBusy:
			c.formState.FocusedField = FieldCalendar
			c.formState.CursorPos = 0
		case FieldConfirm:
			c.formState.FocusedField = FieldFreeBusy
			c.formState.CursorPos = 0
		case FieldCancel:
			c.formState.FocusedField = FieldConfirm
			c.formState.CursorPos = 0
		}

	case " ":
		// Toggle for AllDay and FreeBusy fields, or insert space in text fields
		switch c.formState.FocusedField {
		case FieldAllDay:
			c.formState.AllDayInput = !c.formState.AllDayInput
			if c.formState.AllDayInput {
				c.formState.TimeInput = ""
			}
		case FieldFreeBusy:
			if c.formState.FreeBusyInput == StatusBusy {
				c.formState.FreeBusyInput = StatusFree
			} else {
				c.formState.FreeBusyInput = StatusBusy
			}
		case FieldTitle, FieldDescription, FieldLocation:
			// Insert space in text fields
			c.handleTextInput(" ", msg)
		}

	case "left":
		// Move cursor left in text fields, or toggle for special fields
		switch c.formState.FocusedField {
		case FieldAllDay:
			c.formState.AllDayInput = true
			c.formState.TimeInput = ""
		case FieldFreeBusy:
			c.formState.FreeBusyInput = StatusBusy
		case FieldCalendar:
			c.cycleCalendarPrev()
		case FieldDate, FieldTime, FieldEndTime, FieldTitle, FieldDescription, FieldLocation:
			if c.formState.CursorPos > 0 {
				c.formState.CursorPos--
			}
		}

	case "right":
		// Move cursor right in text fields, or toggle for special fields
		switch c.formState.FocusedField {
		case FieldAllDay:
			c.formState.AllDayInput = false
		case FieldFreeBusy:
			c.formState.FreeBusyInput = StatusFree
		case FieldCalendar:
			c.cycleCalendarNext()
		case FieldDate:
			if c.formState.CursorPos < len(c.formState.DateInput) {
				c.formState.CursorPos++
			}
		case FieldTime:
			if c.formState.CursorPos < len(c.formState.TimeInput) {
				c.formState.CursorPos++
			}
		case FieldEndTime:
			if c.formState.CursorPos < len(c.formState.EndTimeInput) {
				c.formState.CursorPos++
			}
		case FieldTitle:
			if c.formState.CursorPos < len(c.formState.TitleInput) {
				c.formState.CursorPos++
			}
		case FieldDescription:
			if c.formState.CursorPos < len(c.formState.DescriptionInput) {
				c.formState.CursorPos++
			}
		case FieldLocation:
			if c.formState.CursorPos < len(c.formState.LocationInput) {
				c.formState.CursorPos++
			}
		}

	case "enter":
		if c.formState.FocusedField == FieldCancel {
			c.mode = ModeEventList
			c.formState = FormState{}
			c.editingEventIndex = -1
		} else if c.formState.FocusedField == FieldConfirm {
			if c.updateEvent() {
				c.mode = ModeEventList
				c.formState = FormState{}
				c.editingEventIndex = -1
			}
		}

	case "backspace":
		c.handleBackspace()

	default:
		if len(key) == 1 || key == "space" {
			c.handleTextInput(key, msg)
		}
	}

	return c, nil
}

func (c calendarPage) updateDeletePopupMode(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "n", "N":
		c.mode = ModeEventList
		c.formState = FormState{}
		c.editingEventIndex = -1

	case "tab", "left", "right", "h", "l":
		// Toggle between confirm and cancel
		if c.formState.FocusedField == FieldConfirm {
			c.formState.FocusedField = FieldCancel
		} else {
			c.formState.FocusedField = FieldConfirm
		}

	case "enter":
		if c.formState.FocusedField == FieldConfirm {
			c.deleteEvent()
			c.mode = ModeEventList
			c.formState = FormState{}
			c.editingEventIndex = -1
		} else {
			c.mode = ModeEventList
			c.formState = FormState{}
			c.editingEventIndex = -1
		}

	case "y", "Y": // Quick confirm
		c.deleteEvent()
		c.mode = ModeEventList
		c.formState = FormState{}
		c.editingEventIndex = -1
	}

	return c, nil
}

func (c *calendarPage) handleBackspace() {
	switch c.formState.FocusedField {
	case FieldDate:
		if c.formState.CursorPos > 0 && len(c.formState.DateInput) > 0 {
			c.formState.DateInput = c.formState.DateInput[:c.formState.CursorPos-1] + c.formState.DateInput[c.formState.CursorPos:]
			c.formState.CursorPos--
		}
	case FieldTitle:
		if c.formState.CursorPos > 0 && len(c.formState.TitleInput) > 0 {
			c.formState.TitleInput = c.formState.TitleInput[:c.formState.CursorPos-1] + c.formState.TitleInput[c.formState.CursorPos:]
			c.formState.CursorPos--
		}
	case FieldDescription:
		if c.formState.CursorPos > 0 && len(c.formState.DescriptionInput) > 0 {
			c.formState.DescriptionInput = c.formState.DescriptionInput[:c.formState.CursorPos-1] + c.formState.DescriptionInput[c.formState.CursorPos:]
			c.formState.CursorPos--
		}
	case FieldLocation:
		if c.formState.CursorPos > 0 && len(c.formState.LocationInput) > 0 {
			c.formState.LocationInput = c.formState.LocationInput[:c.formState.CursorPos-1] + c.formState.LocationInput[c.formState.CursorPos:]
			c.formState.CursorPos--
		}
	case FieldTime:
		if c.formState.CursorPos > 0 && len(c.formState.TimeInput) > 0 {
			c.formState.TimeInput = c.formState.TimeInput[:c.formState.CursorPos-1] + c.formState.TimeInput[c.formState.CursorPos:]
			c.formState.CursorPos--
		}
	case FieldEndTime:
		if c.formState.CursorPos > 0 && len(c.formState.EndTimeInput) > 0 {
			c.formState.EndTimeInput = c.formState.EndTimeInput[:c.formState.CursorPos-1] + c.formState.EndTimeInput[c.formState.CursorPos:]
			c.formState.CursorPos--
		}
	}
	c.formState.ErrorMsg = "" // Clear error on edit
}

// cycleCalendarNext moves to the next calendar in the list
func (c *calendarPage) cycleCalendarNext() {
	if len(c.calendars) == 0 {
		return
	}
	currentIdx := -1
	for i, cal := range c.calendars {
		if cal.Name == c.formState.CalendarInput {
			currentIdx = i
			break
		}
	}
	nextIdx := (currentIdx + 1) % len(c.calendars)
	// Skip holidays calendar
	if c.calendars[nextIdx].Name == "holidays" {
		nextIdx = (nextIdx + 1) % len(c.calendars)
	}
	c.formState.CalendarInput = c.calendars[nextIdx].Name
}

// cycleCalendarPrev moves to the previous calendar in the list
func (c *calendarPage) cycleCalendarPrev() {
	if len(c.calendars) == 0 {
		return
	}
	currentIdx := -1
	for i, cal := range c.calendars {
		if cal.Name == c.formState.CalendarInput {
			currentIdx = i
			break
		}
	}
	prevIdx := currentIdx - 1
	if prevIdx < 0 {
		prevIdx = len(c.calendars) - 1
	}
	// Skip holidays calendar
	if c.calendars[prevIdx].Name == "holidays" {
		prevIdx--
		if prevIdx < 0 {
			prevIdx = len(c.calendars) - 1
		}
	}
	c.formState.CalendarInput = c.calendars[prevIdx].Name
}

func (c *calendarPage) handleTextInput(key string, msg tea.KeyMsg) {
	char := key
	if key == "space" {
		char = " "
	}

	// Only accept printable characters
	if len(char) != 1 {
		return
	}

	switch c.formState.FocusedField {
	case FieldDate:
		// Only allow digits and dashes for date
		r := rune(char[0])
		if (r >= '0' && r <= '9') || r == '-' {
			if len(c.formState.DateInput) < 10 { // YYYY-MM-DD format
				c.formState.DateInput = c.formState.DateInput[:c.formState.CursorPos] + char + c.formState.DateInput[c.formState.CursorPos:]
				c.formState.CursorPos++
			}
		}
	case FieldTitle:
		if len(c.formState.TitleInput) < 50 {
			c.formState.TitleInput = c.formState.TitleInput[:c.formState.CursorPos] + char + c.formState.TitleInput[c.formState.CursorPos:]
			c.formState.CursorPos++
		}
	case FieldDescription:
		if len(c.formState.DescriptionInput) < 100 {
			c.formState.DescriptionInput = c.formState.DescriptionInput[:c.formState.CursorPos] + char + c.formState.DescriptionInput[c.formState.CursorPos:]
			c.formState.CursorPos++
		}
	case FieldLocation:
		if len(c.formState.LocationInput) < 100 {
			c.formState.LocationInput = c.formState.LocationInput[:c.formState.CursorPos] + char + c.formState.LocationInput[c.formState.CursorPos:]
			c.formState.CursorPos++
		}
	case FieldTime:
		// Only allow digits and colon for time (HH:MM format)
		if !c.formState.AllDayInput {
			r := rune(char[0])
			if (r >= '0' && r <= '9') || r == ':' {
				if len(c.formState.TimeInput) < 5 {
					c.formState.TimeInput = c.formState.TimeInput[:c.formState.CursorPos] + char + c.formState.TimeInput[c.formState.CursorPos:]
					c.formState.CursorPos++
				}
			}
		}
	case FieldEndTime:
		// Only allow digits and colon for end time (HH:MM format)
		if !c.formState.AllDayInput {
			r := rune(char[0])
			if (r >= '0' && r <= '9') || r == ':' {
				if len(c.formState.EndTimeInput) < 5 {
					c.formState.EndTimeInput = c.formState.EndTimeInput[:c.formState.CursorPos] + char + c.formState.EndTimeInput[c.formState.CursorPos:]
					c.formState.CursorPos++
				}
			}
		}
	}
	c.formState.ErrorMsg = "" // Clear error on edit
}