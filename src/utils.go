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

	lipgloss "github.com/charmbracelet/lipgloss"
)

// AppMode represents the current mode of the application
type AppMode int

const (
	ModeNormal AppMode = iota
	ModeEventList
	ModeAddPopup
	ModeEditPopup
	ModeDeletePopup
	ModeMoveEvent
	ModeSearch
)

// FormField represents which field is currently focused in a form
type FormField int

const (
	FieldDate FormField = iota
	FieldTime
	FieldEndTime
	FieldAllDay
	FieldTitle
	FieldDescription
	FieldLocation
	FieldCalendar
	FieldFreeBusy
	FieldConfirm
	FieldCancel
)

// Calendar represents a named calendar with its own color
type Calendar struct {
	Name        string
	DisplayName string
	Color       lipgloss.Color
}

// FreeBusyStatus represents whether the event marks time as busy or free
type FreeBusyStatus int

const (
	StatusBusy FreeBusyStatus = iota
	StatusFree
)

func (s FreeBusyStatus) String() string {
	switch s {
	case StatusFree:
		return "Free"
	default:
		return "Busy"
	}
}

// Event represents a calendar event
type Event struct {
	Date         time.Time
	EndTime      time.Time // End time for duration calculation
	Title        string
	Description  string
	Location     string
	CalendarName string
	AllDay       bool
	Time         string // HH:MM format, empty if AllDay
	FreeBusy     FreeBusyStatus
	UID          string // Unique identifier for ICS
	FilePath     string // Path to the .ics file (if loaded from file)
}

// KeyBinds holds configurable key bindings
type KeyBinds struct {
	PrevMonth []string
	NextMonth []string
	PrevYear  []string
	NextYear  []string
	PrevDay   []string
	NextDay   []string
	Up        []string
	Down      []string
	Left      []string
	Right     []string
	Reset     []string
	Quit      []string
}

// FormState holds the state for add/edit popups
type FormState struct {
	DateInput        string
	TitleInput       string
	DescriptionInput string
	LocationInput    string
	CalendarInput    string
	TimeInput        string
	EndTimeInput     string
	AllDayInput      bool
	FreeBusyInput    FreeBusyStatus
	FocusedField     FormField
	ErrorMsg         string
	CursorPos        int // Cursor position within the current text field
}

// MODEL DATA
type calendarPage struct {
	currDay            int
	currMonth          time.Month
	currYear           int
	selMonth           time.Month
	selYear            int
	selDay             int
	styles             calstyle
	events             []Event
	calendars          []Calendar
	maxEvents          int
	showLegend         bool
	showHolidays       bool
	showWeekNumbers    bool
	defaultCalendar    string
	holidays           []Event
	keybinds           KeyBinds
	eventStyle         lipgloss.Style
	eventListStyle     lipgloss.Style
	mode               AppMode
	selectedEventIndex int
	formState          FormState
	editingEventIndex  int
	movingEventIndex   int
	searchQuery        string
	searchResults      []int // indices into c.events matching search
	searchIndex        int   // currently selected search result
	searchNavigating   bool  // true when navigating results, false when typing
	eventScrollOffset  int   // scroll offset for events list
}

type calstyle struct {
	titleStyle     lipgloss.Style
	headerStyle    lipgloss.Style
	footerStyle    lipgloss.Style
	weekNumStyle   lipgloss.Style
	weekdayStyle   lipgloss.Style
	weekendStyle   lipgloss.Style
	todayStyle     lipgloss.Style
	selectedStyle  lipgloss.Style
	hasEventStyle  lipgloss.Style
	legendStyle    lipgloss.Style
}

func newCalendarPage() calendarPage {
	year, month, day := time.Now().Date()
	keybinds := loadKeybinds()
	maxEvents, showLegend, showHolidays, showWeekNumbers, defaultCalendar, _ := loadDisplayConfig()
	styles := getStyles()
	_, _, _, text, _, _ := getPalette()

	// Try to detect vdirsyncer calendars first
	calendars, vdirEvents := detectVdirsyncerCalendars()

	var events []Event
	// If no vdirsyncer calendars found, fall back to configured or default calendars and events.conf
	if len(calendars) == 0 {
		calendars = loadCalendars()
		// Load local events from events.conf only when no vdirsyncer calendars
		events = loadEvents(calendars)
	} else {
		// Merge any additional configured calendars (for color overrides, display names)
		calendars = mergeCalendarConfigs(calendars)
		// Use vdirsyncer events (don't load events.conf to avoid duplicates)
		events = vdirEvents
	}

	// Generate holidays for current year and next year
	var holidays []Event
	if showHolidays {
		holidays = append(holidays, generateUSHolidays(year)...)
		holidays = append(holidays, generateUSHolidays(year+1)...)
	}

	return calendarPage{
		currDay:      day,
		currMonth:    month,
		currYear:     year,
		selMonth:     month,
		selYear:      year,
		selDay:       day,
		styles:       styles,
		events:       events,
		calendars:    calendars,
		maxEvents:       maxEvents,
		showLegend:      showLegend,
		showHolidays:    showHolidays,
		showWeekNumbers: showWeekNumbers,
		defaultCalendar: defaultCalendar,
		holidays:        holidays,
		keybinds:     keybinds,
		eventStyle: lipgloss.NewStyle().
			Foreground(text).
			PaddingLeft(1),
		eventListStyle: lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(text).
			PaddingLeft(1).
			MarginLeft(2),
		mode:               ModeNormal,
		selectedEventIndex: 0,
		formState:          FormState{},
		editingEventIndex:  -1,
		movingEventIndex:   -1,
	}
}

// moveEventToSelectedDay moves the event at movingEventIndex to the currently selected day
func (c *calendarPage) moveEventToSelectedDay() bool {
	if c.movingEventIndex < 0 || c.movingEventIndex >= len(c.events) {
		return false
	}

	// Update the event's date to the selected day
	event := &c.events[c.movingEventIndex]
	newDate := time.Date(c.selYear, c.selMonth, c.selDay,
		event.Date.Hour(), event.Date.Minute(), event.Date.Second(), 0, time.Local)
	event.Date = newDate

	c.sortEvents()
	c.saveEvents()
	c.movingEventIndex = -1
	return true
}

func (c *calendarPage) initAddForm() {
	// Pre-populate with selected date and default calendar
	defaultCal := ""
	if c.defaultCalendar != "" {
		// Check if configured default calendar exists
		for _, cal := range c.calendars {
			if cal.Name == c.defaultCalendar {
				defaultCal = c.defaultCalendar
				break
			}
		}
	}
	// Fall back to first calendar if default not set or not found
	if defaultCal == "" && len(c.calendars) > 0 {
		defaultCal = c.calendars[0].Name
	}
	dateStr := time.Date(c.selYear, c.selMonth, c.selDay, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	c.formState = FormState{
		DateInput:        dateStr,
		TitleInput:       "",
		DescriptionInput: "",
		LocationInput:    "",
		CalendarInput:    defaultCal,
		TimeInput:        "",
		EndTimeInput:     "",
		AllDayInput:      true,
		FreeBusyInput:    StatusBusy,
		FocusedField:     FieldTitle,
		ErrorMsg:         "",
		CursorPos:        0,
	}
}

func (c *calendarPage) initEditForm(eventIndex int) {
	if eventIndex < 0 || eventIndex >= len(c.events) {
		return
	}
	event := c.events[eventIndex]
	c.editingEventIndex = eventIndex
	// Format end time if available
	endTimeStr := ""
	if !event.EndTime.IsZero() && !event.AllDay {
		endTimeStr = event.EndTime.Format("15:04")
	}
	c.formState = FormState{
		DateInput:        event.Date.Format("2006-01-02"),
		TitleInput:       event.Title,
		DescriptionInput: event.Description,
		LocationInput:    event.Location,
		CalendarInput:    event.CalendarName,
		TimeInput:        event.Time,
		EndTimeInput:     endTimeStr,
		AllDayInput:      event.AllDay,
		FreeBusyInput:    event.FreeBusy,
		FocusedField:     FieldTitle,
		ErrorMsg:         "",
		CursorPos:        len(event.Title),
	}
}

func (c *calendarPage) initDeleteConfirm(eventIndex int) {
	if eventIndex < 0 || eventIndex >= len(c.events) {
		return
	}
	c.editingEventIndex = eventIndex
	c.formState = FormState{
		FocusedField: FieldConfirm,
	}
}

func (c *calendarPage) addEvent() bool {
	date, err := time.Parse("2006-01-02", c.formState.DateInput)
	if err != nil {
		c.formState.ErrorMsg = "Invalid date format (use YYYY-MM-DD)"
		return false
	}
	if strings.TrimSpace(c.formState.TitleInput) == "" {
		c.formState.ErrorMsg = "Title cannot be empty"
		return false
	}

	calName := strings.TrimSpace(c.formState.CalendarInput)
	if calName == "" && len(c.calendars) > 0 {
		calName = c.calendars[0].Name
	}

	// Validate time format if not all-day
	timeStr := strings.TrimSpace(c.formState.TimeInput)
	if !c.formState.AllDayInput && timeStr != "" {
		if !isValidTimeFormat(timeStr) {
			c.formState.ErrorMsg = "Invalid time format (use HH:MM)"
			return false
		}
	}

	// Validate and parse end time
	endTimeStr := strings.TrimSpace(c.formState.EndTimeInput)
	var endTime time.Time
	if !c.formState.AllDayInput && endTimeStr != "" {
		if !isValidTimeFormat(endTimeStr) {
			c.formState.ErrorMsg = "Invalid end time format (use HH:MM)"
			return false
		}
		// Parse end time and combine with date
		parts := strings.Split(endTimeStr, ":")
		if len(parts) == 2 {
			var hour, minute int
			fmt.Sscanf(parts[0], "%d", &hour)
			fmt.Sscanf(parts[1], "%d", &minute)
			endTime = time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, time.Local)
		}
	}

	newEvent := Event{
		Date:         date,
		EndTime:      endTime,
		Title:        strings.TrimSpace(c.formState.TitleInput),
		Description:  strings.TrimSpace(c.formState.DescriptionInput),
		Location:     strings.TrimSpace(c.formState.LocationInput),
		CalendarName: calName,
		AllDay:       c.formState.AllDayInput,
		Time:         timeStr,
		FreeBusy:     c.formState.FreeBusyInput,
	}

	// Write to ICS file (CalDAV sync)
	if err := WriteEventToICS(&newEvent); err != nil {
		c.formState.ErrorMsg = "Failed to save event: " + err.Error()
		return false
	}

	c.events = append(c.events, newEvent)
	c.sortEvents()

	// Trigger vdirsyncer sync in background
	RunVdirsyncerSync()

	return true
}

func (c *calendarPage) updateEvent() bool {
	if c.editingEventIndex < 0 || c.editingEventIndex >= len(c.events) {
		return false
	}

	date, err := time.Parse("2006-01-02", c.formState.DateInput)
	if err != nil {
		c.formState.ErrorMsg = "Invalid date format (use YYYY-MM-DD)"
		return false
	}
	if strings.TrimSpace(c.formState.TitleInput) == "" {
		c.formState.ErrorMsg = "Title cannot be empty"
		return false
	}

	calName := strings.TrimSpace(c.formState.CalendarInput)
	if calName == "" && len(c.calendars) > 0 {
		calName = c.calendars[0].Name
	}

	// Validate time format if not all-day
	timeStr := strings.TrimSpace(c.formState.TimeInput)
	if !c.formState.AllDayInput && timeStr != "" {
		if !isValidTimeFormat(timeStr) {
			c.formState.ErrorMsg = "Invalid time format (use HH:MM)"
			return false
		}
	}

	// Validate and parse end time
	endTimeStr := strings.TrimSpace(c.formState.EndTimeInput)
	var endTime time.Time
	if !c.formState.AllDayInput && endTimeStr != "" {
		if !isValidTimeFormat(endTimeStr) {
			c.formState.ErrorMsg = "Invalid end time format (use HH:MM)"
			return false
		}
		// Parse end time and combine with date
		parts := strings.Split(endTimeStr, ":")
		if len(parts) == 2 {
			var hour, minute int
			fmt.Sscanf(parts[0], "%d", &hour)
			fmt.Sscanf(parts[1], "%d", &minute)
			endTime = time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, time.Local)
		}
	}

	// Preserve UID and FilePath from existing event
	existingEvent := c.events[c.editingEventIndex]

	updatedEvent := Event{
		Date:         date,
		EndTime:      endTime,
		Title:        strings.TrimSpace(c.formState.TitleInput),
		Description:  strings.TrimSpace(c.formState.DescriptionInput),
		Location:     strings.TrimSpace(c.formState.LocationInput),
		CalendarName: calName,
		AllDay:       c.formState.AllDayInput,
		Time:         timeStr,
		FreeBusy:     c.formState.FreeBusyInput,
		UID:          existingEvent.UID,
		FilePath:     existingEvent.FilePath,
	}

	// Write to ICS file (CalDAV sync)
	if updatedEvent.FilePath != "" && updatedEvent.UID != "" {
		if err := UpdateEventICS(&updatedEvent); err != nil {
			c.formState.ErrorMsg = "Failed to update event: " + err.Error()
			return false
		}
	}

	c.events[c.editingEventIndex] = updatedEvent
	c.sortEvents()

	// Trigger vdirsyncer sync in background
	RunVdirsyncerSync()

	return true
}

func (c *calendarPage) deleteEvent() bool {
	if c.editingEventIndex < 0 || c.editingEventIndex >= len(c.events) {
		return false
	}

	// Delete the ICS file (CalDAV sync)
	event := c.events[c.editingEventIndex]
	if event.FilePath != "" {
		DeleteEventICS(&event)
	}

	// Remove the event at the index
	c.events = append(c.events[:c.editingEventIndex], c.events[c.editingEventIndex+1:]...)

	// Adjust selected index if needed
	if c.selectedEventIndex >= len(c.events) && c.selectedEventIndex > 0 {
		c.selectedEventIndex--
	}

	// Trigger vdirsyncer sync in background
	RunVdirsyncerSync()

	return true
}

func (c *calendarPage) sortEvents() {
	sort.Slice(c.events, func(i, j int) bool {
		return c.events[i].Date.Before(c.events[j].Date)
	})
}

// performSearch searches events by title and description, storing matching indices
func (c *calendarPage) performSearch() {
	c.searchResults = nil
	c.searchIndex = 0

	if strings.TrimSpace(c.searchQuery) == "" {
		return
	}

	query := strings.ToLower(c.searchQuery)
	for i, event := range c.events {
		titleMatch := strings.Contains(strings.ToLower(event.Title), query)
		descMatch := strings.Contains(strings.ToLower(event.Description), query)
		if titleMatch || descMatch {
			c.searchResults = append(c.searchResults, i)
		}
	}
}

// navigateToSearchResult jumps to the currently selected search result
func (c *calendarPage) navigateToSearchResult() {
	if len(c.searchResults) == 0 || c.searchIndex >= len(c.searchResults) {
		return
	}

	eventIndex := c.searchResults[c.searchIndex]
	if eventIndex >= 0 && eventIndex < len(c.events) {
		event := c.events[eventIndex]
		// Navigate calendar to the event's date
		c.selYear = event.Date.Year()
		c.selMonth = event.Date.Month()
		c.selDay = event.Date.Day()
	}
}

func (c *calendarPage) saveEvents() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	configDir := filepath.Join(homeDir, ".config", "zen-cal")
	// Ensure directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return
	}

	eventsPath := filepath.Join(configDir, "events.conf")
	file, err := os.Create(eventsPath)
	if err != nil {
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("# Zen-Cal Events\n")
	writer.WriteString("# Format: YYYY-MM-DD | Time | Title | Description | Calendar | FreeBusy\n")
	writer.WriteString("# Time: HH:MM or 'all-day'\n")
	writer.WriteString("# FreeBusy: 'busy' or 'free'\n\n")

	for _, event := range c.events {
		timeStr := "all-day"
		if !event.AllDay && event.Time != "" {
			timeStr = event.Time
		}
		freeBusyStr := "busy"
		if event.FreeBusy == StatusFree {
			freeBusyStr = "free"
		}
		line := fmt.Sprintf("%s | %s | %s | %s | %s | %s",
			event.Date.Format("2006-01-02"),
			timeStr,
			event.Title,
			event.Description,
			event.CalendarName,
			freeBusyStr,
		)
		writer.WriteString(line + "\n")
	}

	writer.Flush()
}

func loadDisplayConfig() (int, bool, bool, bool, string, int) {
	maxEvents := 5            // default
	showLegend := true        // default
	showHolidays := false     // default
	showWeekNumbers := true   // default
	defaultCalendar := ""     // default (empty means use first calendar)
	eventIndicatorDays := 0   // default (0 = day of event only)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return maxEvents, showLegend, showHolidays, showWeekNumbers, defaultCalendar, eventIndicatorDays
	}
	configPath := filepath.Join(homeDir, ".config", "zen-cal", "zen-cal.conf")
	file, err := os.Open(configPath)
	if err != nil {
		return maxEvents, showLegend, showHolidays, showWeekNumbers, defaultCalendar, eventIndicatorDays
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

		switch key {
		case "max_events":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				maxEvents = n
			}
		case "show_legend":
			val = strings.ToLower(val)
			showLegend = val == "true" || val == "1" || val == "yes"
		case "show_holidays":
			val = strings.ToLower(val)
			showHolidays = val == "true" || val == "1" || val == "yes"
		case "show_week_numbers":
			val = strings.ToLower(val)
			showWeekNumbers = val == "true" || val == "1" || val == "yes"
		case "default_calendar":
			defaultCalendar = val
		case "event_indicator_days":
			if n, err := strconv.Atoi(val); err == nil && n >= 0 {
				eventIndicatorDays = n
			}
		}
	}

	return maxEvents, showLegend, showHolidays, showWeekNumbers, defaultCalendar, eventIndicatorDays
}

func loadCalendars() []Calendar {
	var calendars []Calendar
	hexColor := regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return defaultCalendars()
	}
	configPath := filepath.Join(homeDir, ".config", "zen-cal", "zen-cal.conf")
	file, err := os.Open(configPath)
	if err != nil {
		return defaultCalendars()
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

		// Calendar format: calendar.<name> = #color | Display Name
		if strings.HasPrefix(key, "calendar.") {
			calName := strings.TrimPrefix(key, "calendar.")
			if calName == "" {
				continue
			}

			// Parse color and optional display name
			valParts := strings.SplitN(val, "|", 2)
			colorStr := strings.TrimSpace(valParts[0])
			displayName := calName // Default to code name

			if len(valParts) == 2 {
				displayName = strings.TrimSpace(valParts[1])
			}

			if hexColor.MatchString(colorStr) {
				calendars = append(calendars, Calendar{
					Name:        calName,
					DisplayName: displayName,
					Color:       lipgloss.Color(colorStr),
				})
			}
		}
	}

	if len(calendars) == 0 {
		return defaultCalendars()
	}

	return calendars
}

func defaultCalendars() []Calendar {
	return []Calendar{
		{Name: "personal", DisplayName: "Personal", Color: lipgloss.Color("#f38ba8")},
		{Name: "work", DisplayName: "Work", Color: lipgloss.Color("#89b4fa")},
		{Name: "family", DisplayName: "Family", Color: lipgloss.Color("#a6e3a1")},
		{Name: "holidays", DisplayName: "Holidays", Color: lipgloss.Color("#f9e2af")},
	}
}

// mergeCalendarConfigs merges vdirsyncer-detected calendars with zen-cal config overrides
func mergeCalendarConfigs(vdirCalendars []Calendar) []Calendar {
	// Load config overrides for colors and display names
	configCalendars := loadCalendars()
	configMap := make(map[string]Calendar)
	for _, cal := range configCalendars {
		configMap[cal.Name] = cal
	}

	// Apply overrides to vdirsyncer calendars
	result := make([]Calendar, 0, len(vdirCalendars))
	for _, vdirCal := range vdirCalendars {
		if configCal, ok := configMap[vdirCal.Name]; ok {
			// Override color if specified in config
			if configCal.Color != "" {
				vdirCal.Color = configCal.Color
			}
			// Override display name if specified in config
			if configCal.DisplayName != "" && configCal.DisplayName != configCal.Name {
				vdirCal.DisplayName = configCal.DisplayName
			}
		}
		result = append(result, vdirCal)
	}

	// Always ensure holidays calendar exists
	hasHolidays := false
	for _, cal := range result {
		if cal.Name == "holidays" {
			hasHolidays = true
			break
		}
	}
	if !hasHolidays {
		result = append(result, Calendar{
			Name:        "holidays",
			DisplayName: "Holidays",
			Color:       lipgloss.Color("#f9e2af"),
		})
	}

	return result
}

// generateShiftKeys creates shift+key versions of the given keys
func generateShiftKeys(keys []string) []string {
	var shiftKeys []string
	for _, key := range keys {
		// Skip if already has shift
		if strings.HasPrefix(key, "shift+") {
			shiftKeys = append(shiftKeys, key)
			continue
		}
		// For single letters, use uppercase
		if len(key) == 1 && key >= "a" && key <= "z" {
			shiftKeys = append(shiftKeys, strings.ToUpper(key))
		} else {
			// For other keys (like arrows), add shift+ prefix
			shiftKeys = append(shiftKeys, "shift+"+key)
		}
	}
	return shiftKeys
}

func loadKeybinds() KeyBinds {
	// Default keybinds - arrow keys move day, shift+arrows change month/year
	keybinds := KeyBinds{
		PrevMonth: []string{},
		NextMonth: []string{},
		PrevYear:  []string{},
		NextYear:  []string{},
		PrevDay:   []string{"left", "h"},
		NextDay:   []string{"right", "l"},
		Up:        []string{"up", "k"},
		Down:      []string{"down", "j"},
		Left:      []string{"left", "h"},
		Right:     []string{"right", "l"},
		Reset:     []string{"r"},
		Quit:      []string{"ctrl+c", "q", "esc"},
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Auto-generate shift+key combinations for month/year navigation
		keybinds.PrevMonth = generateShiftKeys(keybinds.Left)
		keybinds.NextMonth = generateShiftKeys(keybinds.Right)
		keybinds.PrevYear = generateShiftKeys(keybinds.Up)
		keybinds.NextYear = generateShiftKeys(keybinds.Down)
		return keybinds
	}

	configPath := filepath.Join(homeDir, ".config", "zen-cal", "zen-cal.conf")
	file, err := os.Open(configPath)
	if err != nil {
		return keybinds
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

		// Parse comma-separated keybinds
		keys := parseKeyList(val)
		if len(keys) == 0 {
			continue
		}

		switch key {
		case "key_prev_month":
			keybinds.PrevMonth = keys
		case "key_next_month":
			keybinds.NextMonth = keys
		case "key_prev_year":
			keybinds.PrevYear = keys
		case "key_next_year":
			keybinds.NextYear = keys
		case "key_prev_day":
			keybinds.PrevDay = keys
		case "key_next_day":
			keybinds.NextDay = keys
		case "key_up":
			keybinds.Up = keys
		case "key_down":
			keybinds.Down = keys
		case "key_left":
			keybinds.Left = keys
		case "key_right":
			keybinds.Right = keys
		case "key_reset":
			keybinds.Reset = keys
		case "key_quit":
			keybinds.Quit = keys
		}
	}

	// Auto-generate shift+key combinations for month/year navigation
	// based on the direction keys (after loading any custom keys)
	if len(keybinds.PrevMonth) == 0 {
		keybinds.PrevMonth = generateShiftKeys(keybinds.Left)
	}
	if len(keybinds.NextMonth) == 0 {
		keybinds.NextMonth = generateShiftKeys(keybinds.Right)
	}
	if len(keybinds.PrevYear) == 0 {
		keybinds.PrevYear = generateShiftKeys(keybinds.Up)
	}
	if len(keybinds.NextYear) == 0 {
		keybinds.NextYear = generateShiftKeys(keybinds.Down)
	}

	return keybinds
}

func parseKeyList(val string) []string {
	var keys []string
	for _, k := range strings.Split(val, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

func loadEvents(calendars []Calendar) []Event {
	var events []Event

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return events
	}
	eventsPath := filepath.Join(homeDir, ".config", "zen-cal", "events.conf")
	file, err := os.Open(eventsPath)
	if err != nil {
		return events
	}
	defer file.Close()

	// Get default calendar name
	defaultCal := ""
	if len(calendars) > 0 {
		defaultCal = calendars[0].Name
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Format: YYYY-MM-DD | Time | Title | Description | Calendar | FreeBusy
			parts := strings.SplitN(line, "|", 6)
			if len(parts) < 3 {
				continue
			}

			dateStr := strings.TrimSpace(parts[0])
			timeStr := strings.TrimSpace(parts[1])
			title := strings.TrimSpace(parts[2])
			description := ""
			calendarName := defaultCal
			freeBusy := StatusBusy
			allDay := timeStr == "" || strings.ToLower(timeStr) == "all-day"

			if len(parts) >= 4 {
				description = strings.TrimSpace(parts[3])
			}
			if len(parts) >= 5 {
				calendarName = strings.TrimSpace(parts[4])
			}
			if len(parts) >= 6 {
				fbStr := strings.ToLower(strings.TrimSpace(parts[5]))
				if fbStr == "free" {
					freeBusy = StatusFree
				}
			}

			date, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				continue
			}

			// Clean up time string
			if allDay {
				timeStr = ""
			}

			events = append(events, Event{
				Date:         date,
				Title:        title,
				Description:  description,
				CalendarName: calendarName,
				AllDay:       allDay,
				Time:         timeStr,
				FreeBusy:     freeBusy,
			})
	}

	// Sort events by date
	sort.Slice(events, func(i, j int) bool {
		return events[i].Date.Before(events[j].Date)
	})

	return events
}

// GetUpcomingEvents returns events starting from the selected day that haven't passed
func (c *calendarPage) GetUpcomingEvents() []Event {
	var upcoming []Event

	// Create reference date from selected day/month/year
	selectedDate := time.Date(c.selYear, c.selMonth, c.selDay, 0, 0, 0, 0, time.Local)

	// Combine user events and holidays
	allEvents := make([]Event, 0, len(c.events)+len(c.holidays))
	allEvents = append(allEvents, c.events...)
	if c.showHolidays {
		allEvents = append(allEvents, c.holidays...)
	}

	// Sort combined events by date
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].Date.Before(allEvents[j].Date)
	})

	for _, event := range allEvents {
		// Include events on or after the selected date
		eventDate := time.Date(event.Date.Year(), event.Date.Month(), event.Date.Day(), 0, 0, 0, 0, time.Local)
		if !eventDate.Before(selectedDate) {
			upcoming = append(upcoming, event)
			if len(upcoming) >= c.maxEvents {
				break
			}
		}
	}

	return upcoming
}

// GetUpcomingEventsWithIndices returns indices into c.events that match the order of GetUpcomingEvents()
// Returns -1 for holidays (which cannot be edited)
func (c *calendarPage) GetUpcomingEventsWithIndices() []int {
	var indices []int

	selectedDate := time.Date(c.selYear, c.selMonth, c.selDay, 0, 0, 0, 0, time.Local)

	// Build a list of events with their original indices, matching GetUpcomingEvents logic
	type eventWithIndex struct {
		event Event
		index int // -1 for holidays
	}
	var allEvents []eventWithIndex

	// Add user events with their indices
	for i, event := range c.events {
		allEvents = append(allEvents, eventWithIndex{event: event, index: i})
	}

	// Add holidays with index -1
	if c.showHolidays {
		for _, event := range c.holidays {
			allEvents = append(allEvents, eventWithIndex{event: event, index: -1})
		}
	}

	// Sort combined events by date (same as GetUpcomingEvents)
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].event.Date.Before(allEvents[j].event.Date)
	})

	// Filter to upcoming events and collect indices
	for _, e := range allEvents {
		eventDate := time.Date(e.event.Date.Year(), e.event.Date.Month(), e.event.Date.Day(), 0, 0, 0, 0, time.Local)
		if !eventDate.Before(selectedDate) {
			indices = append(indices, e.index)
			if len(indices) >= c.maxEvents {
				break
			}
		}
	}

	return indices
}

// containsKey checks if a key is in the list of keybinds
func containsKey(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

func getMonthInfo(month time.Month, year int) (time.Weekday, int, int) {
	firstDay := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	firstWeekDay := firstDay.Weekday() // Sunday = 0
	lastDay := firstDay.AddDate(0, 1, -1).Day()
	jan1 := time.Date(year, time.January, 1, 0, 0, 0, 0, time.Local)
	week := (firstDay.YearDay()+int(jan1.Weekday())-1)/7 + 1
	return firstWeekDay, week, lastDay
}

func getPalette() (today, today_text, headings, text, weekends, keys lipgloss.Color) {
	today = lipgloss.Color("#f38ba8")      // Accent / highlight background
	today_text = lipgloss.Color("#cdd6f4") // highlight text
	headings = lipgloss.Color("#cba6f7")   // Subtle / dim text
	text = lipgloss.Color("#cdd6f4")       // Main text / foreground
	weekends = lipgloss.Color("#f9e2af")   // Special highlight (weekend / weekends)
	keys = lipgloss.Color("#89dceb")       // Keybind highlight color
	hexColor := regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return // Return defaults if home directory cannot be determined
	}
	confFileName := "zen-cal.conf"
	configPath := filepath.Join(homeDir, ".config", "zen-cal", confFileName)
	file, err := os.Open(configPath)
	if err != nil {
		return // Return defaults if config file doesn't exist
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

		if !hexColor.MatchString(val) {
			continue // skip invalid hex codes
		}
		switch key {
		case "today":
			today = lipgloss.Color(val)
		case "today_text":
			today_text = lipgloss.Color(val)
		case "headings":
			headings = lipgloss.Color(val)
		case "text":
			text = lipgloss.Color(val)
		case "weekends":
			weekends = lipgloss.Color(val)
		case "keys":
			keys = lipgloss.Color(val)
		}
	}

	if err := scanner.Err(); err != nil {
		return
	}

	return
}

func getStyles() calstyle {
	today, today_text, headings, text, weekends, _ := getPalette()

	// Base cell style
	cellBase := lipgloss.NewStyle().
		Width(4).
		Align(lipgloss.Center)

	return calstyle{
		// text / main headings
		titleStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(text),

		// Footer
		footerStyle: lipgloss.NewStyle().
			Foreground(text),

		// Header / index / row numbers
		headerStyle: cellBase.
			Foreground(headings).
			Italic(true),

		weekNumStyle: cellBase.
			Foreground(headings).
			Italic(true),

		// Weekday cells
		weekdayStyle: cellBase.
			Foreground(text),

		// Weekend / weekends cells
		weekendStyle: cellBase.
			Foreground(weekends),

		// Current day / active selection
		todayStyle: cellBase.
			Foreground(today_text).
			Background(today).
			Bold(true),

		// Selected day (when navigating)
		selectedStyle: cellBase.
			Foreground(text).
			Background(headings).
			Bold(true),

		// Day with event(s)
		hasEventStyle: cellBase.
			Foreground(weekends).
			Underline(true),

		// Legend style
		legendStyle: lipgloss.NewStyle().
			Foreground(text).
			Faint(true),
	}
}

// getDaysInMonth returns the number of days in a given month/year
func getDaysInMonth(month time.Month, year int) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()
}

// generateUSHolidays generates US federal holidays for a given year
func generateUSHolidays(year int) []Event {
	holidays := []Event{
		// Fixed date holidays
		{
			Date:         time.Date(year, time.January, 1, 0, 0, 0, 0, time.Local),
			Title:        "New Year's Day",
			AllDay:       true,
			FreeBusy:     StatusFree,
			CalendarName: "holidays",
		},
		{
			Date:         time.Date(year, time.June, 19, 0, 0, 0, 0, time.Local),
			Title:        "Juneteenth",
			AllDay:       true,
			FreeBusy:     StatusFree,
			CalendarName: "holidays",
		},
		{
			Date:         time.Date(year, time.July, 4, 0, 0, 0, 0, time.Local),
			Title:        "Independence Day",
			AllDay:       true,
			FreeBusy:     StatusFree,
			CalendarName: "holidays",
		},
		{
			Date:         time.Date(year, time.November, 11, 0, 0, 0, 0, time.Local),
			Title:        "Veterans Day",
			AllDay:       true,
			FreeBusy:     StatusFree,
			CalendarName: "holidays",
		},
		{
			Date:         time.Date(year, time.December, 25, 0, 0, 0, 0, time.Local),
			Title:        "Christmas Day",
			AllDay:       true,
			FreeBusy:     StatusFree,
			CalendarName: "holidays",
		},

		// Floating holidays
		{
			Date:         nthWeekdayOfMonth(year, time.January, time.Monday, 3),
			Title:        "Martin Luther King Jr. Day",
			AllDay:       true,
			FreeBusy:     StatusFree,
			CalendarName: "holidays",
		},
		{
			Date:         nthWeekdayOfMonth(year, time.February, time.Monday, 3),
			Title:        "Presidents' Day",
			AllDay:       true,
			FreeBusy:     StatusFree,
			CalendarName: "holidays",
		},
		{
			Date:         lastWeekdayOfMonth(year, time.May, time.Monday),
			Title:        "Memorial Day",
			AllDay:       true,
			FreeBusy:     StatusFree,
			CalendarName: "holidays",
		},
		{
			Date:         nthWeekdayOfMonth(year, time.September, time.Monday, 1),
			Title:        "Labor Day",
			AllDay:       true,
			FreeBusy:     StatusFree,
			CalendarName: "holidays",
		},
		{
			Date:         nthWeekdayOfMonth(year, time.October, time.Monday, 2),
			Title:        "Columbus Day",
			AllDay:       true,
			FreeBusy:     StatusFree,
			CalendarName: "holidays",
		},
		{
			Date:         nthWeekdayOfMonth(year, time.November, time.Thursday, 4),
			Title:        "Thanksgiving Day",
			AllDay:       true,
			FreeBusy:     StatusFree,
			CalendarName: "holidays",
		},
	}

	return holidays
}

// nthWeekdayOfMonth returns the date of the nth occurrence of a weekday in a month
func nthWeekdayOfMonth(year int, month time.Month, weekday time.Weekday, n int) time.Time {
	// Start from the first day of the month
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)

	// Find the first occurrence of the weekday
	daysUntil := int(weekday - first.Weekday())
	if daysUntil < 0 {
		daysUntil += 7
	}

	// Add (n-1) weeks to get to the nth occurrence
	return first.AddDate(0, 0, daysUntil+(n-1)*7)
}

// lastWeekdayOfMonth returns the date of the last occurrence of a weekday in a month
func lastWeekdayOfMonth(year int, month time.Month, weekday time.Weekday) time.Time {
	// Start from the last day of the month
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local)

	// Find how many days back to the last occurrence of weekday
	daysBack := int(last.Weekday() - weekday)
	if daysBack < 0 {
		daysBack += 7
	}

	return last.AddDate(0, 0, -daysBack)
}

// isValidTimeFormat checks if a time string is in HH:MM format
func isValidTimeFormat(timeStr string) bool {
	if len(timeStr) != 5 {
		return false
	}
	if timeStr[2] != ':' {
		return false
	}
	hour, err1 := strconv.Atoi(timeStr[0:2])
	min, err2 := strconv.Atoi(timeStr[3:5])
	if err1 != nil || err2 != nil {
		return false
	}
	return hour >= 0 && hour <= 23 && min >= 0 && min <= 59
}

// getEventCalendarOnDay returns the calendar for an event on the given day, or nil if no event
func (c *calendarPage) getEventCalendarOnDay(day int, month time.Month, year int) *Calendar {
	// Check user events first
	for _, event := range c.events {
		if event.Date.Day() == day &&
			event.Date.Month() == month &&
			event.Date.Year() == year {
			// Find the calendar for this event
			for i := range c.calendars {
				if c.calendars[i].Name == event.CalendarName {
					return &c.calendars[i]
				}
			}
			// Return first calendar as default if not found
			if len(c.calendars) > 0 {
				return &c.calendars[0]
			}
		}
	}

	// Check holidays
	if c.showHolidays {
		for _, holiday := range c.holidays {
			if holiday.Date.Day() == day &&
				holiday.Date.Month() == month &&
				holiday.Date.Year() == year {
				// Return holiday calendar
				for i := range c.calendars {
					if c.calendars[i].Name == "holidays" {
						return &c.calendars[i]
					}
				}
			}
		}
	}

	return nil
}

// isHoliday checks if an event is a holiday (not a user event)
func (c *calendarPage) isHoliday(event Event) bool {
	return event.CalendarName == "holidays"
}

// getCalendarByName returns a calendar by name or nil if not found
func (c *calendarPage) getCalendarByName(name string) *Calendar {
	for i := range c.calendars {
		if c.calendars[i].Name == name {
			return &c.calendars[i]
		}
	}
	return nil
}