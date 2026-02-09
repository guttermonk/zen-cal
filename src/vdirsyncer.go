package main

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	lipgloss "github.com/charmbracelet/lipgloss"
)

// pendingSyncCount tracks how many syncs are pending/running
var pendingSyncCount int

// RunVdirsyncerSync runs vdirsyncer sync in the background
// It's designed to be called after any event modification
func RunVdirsyncerSync() {
	pendingSyncCount++
	go func() {
		defer func() { pendingSyncCount-- }()
		cmd := exec.Command("vdirsyncer", "sync")
		cmd.Stdout = nil
		cmd.Stderr = nil
		_ = cmd.Run() // Ignore errors - sync failures shouldn't block the user
	}()
}

// RunVdirsyncerSyncBlocking runs vdirsyncer sync and waits for completion
// Used when exiting the program to ensure all changes are synced
func RunVdirsyncerSyncBlocking() {
	cmd := exec.Command("vdirsyncer", "sync")
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Run()
}

// HasPendingSync returns true if there are pending sync operations
func HasPendingSync() bool {
	return pendingSyncCount > 0
}

// generateUID creates a unique identifier for ICS events
func generateUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// getCalendarPath returns the filesystem path for a calendar by name
func getCalendarPath(calendarName string) string {
	storagePaths := findVdirsyncerStoragePaths()
	for _, storagePath := range storagePaths {
		calPath := filepath.Join(storagePath, calendarName)
		if info, err := os.Stat(calPath); err == nil && info.IsDir() {
			return calPath
		}
	}
	return ""
}

// WriteEventToICS writes an event to an ICS file in the appropriate calendar directory
func WriteEventToICS(event *Event) error {
	calPath := getCalendarPath(event.CalendarName)
	if calPath == "" {
		return fmt.Errorf("calendar path not found for: %s", event.CalendarName)
	}

	// Generate UID if not set
	if event.UID == "" {
		event.UID = generateUID()
	}

	// Determine file path
	icsPath := filepath.Join(calPath, event.UID+".ics")
	event.FilePath = icsPath

	// Create ICS content
	icsContent := buildICSContent(event)

	// Write to file
	err := os.WriteFile(icsPath, []byte(icsContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write ICS file: %w", err)
	}

	return nil
}

// UpdateEventICS updates an existing ICS file
func UpdateEventICS(event *Event) error {
	if event.FilePath == "" || event.UID == "" {
		return fmt.Errorf("event has no file path or UID")
	}

	// Rebuild ICS content
	icsContent := buildICSContent(event)

	// Write to file
	err := os.WriteFile(event.FilePath, []byte(icsContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to update ICS file: %w", err)
	}

	return nil
}

// DeleteEventICS deletes an ICS file
func DeleteEventICS(event *Event) error {
	if event.FilePath == "" {
		return fmt.Errorf("event has no file path")
	}

	err := os.Remove(event.FilePath)
	if err != nil {
		return fmt.Errorf("failed to delete ICS file: %w", err)
	}

	return nil
}

// buildICSContent creates the ICS file content for an event
func buildICSContent(event *Event) string {
	var sb strings.Builder
	now := time.Now().UTC()
	dtstamp := now.Format("20060102T150405Z")

	sb.WriteString("BEGIN:VCALENDAR\r\n")
	sb.WriteString("VERSION:2.0\r\n")
	sb.WriteString("PRODID:-//zen-cal//EN\r\n")

	// Add timezone definition for non-UTC times
	if !event.AllDay {
		sb.WriteString("BEGIN:VTIMEZONE\r\n")
		sb.WriteString("TZID:Local\r\n")
		sb.WriteString("BEGIN:STANDARD\r\n")
		sb.WriteString("DTSTART:19700101T000000\r\n")
		sb.WriteString("TZOFFSETFROM:+0000\r\n")
		sb.WriteString("TZOFFSETTO:+0000\r\n")
		sb.WriteString("END:STANDARD\r\n")
		sb.WriteString("END:VTIMEZONE\r\n")
	}

	sb.WriteString("BEGIN:VEVENT\r\n")
	sb.WriteString(fmt.Sprintf("UID:%s\r\n", event.UID))
	sb.WriteString(fmt.Sprintf("DTSTAMP:%s\r\n", dtstamp))
	sb.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", escapeICSValue(event.Title)))

	// DTSTART and DTEND
	if event.AllDay {
		dtstart := event.Date.Format("20060102")
		sb.WriteString(fmt.Sprintf("DTSTART;VALUE=DATE:%s\r\n", dtstart))
		// For all-day events, DTEND is the next day
		endDate := event.Date.AddDate(0, 0, 1)
		if !event.EndTime.IsZero() {
			endDate = event.EndTime
		}
		dtend := endDate.Format("20060102")
		sb.WriteString(fmt.Sprintf("DTEND;VALUE=DATE:%s\r\n", dtend))
	} else {
		// Parse time and combine with date
		var startDateTime time.Time
		if event.Time != "" {
			parts := strings.Split(event.Time, ":")
			if len(parts) == 2 {
				hour := 0
				minute := 0
				fmt.Sscanf(parts[0], "%d", &hour)
				fmt.Sscanf(parts[1], "%d", &minute)
				startDateTime = time.Date(event.Date.Year(), event.Date.Month(), event.Date.Day(),
					hour, minute, 0, 0, time.Local)
			}
		} else {
			startDateTime = event.Date
		}
		dtstart := startDateTime.Format("20060102T150405")
		sb.WriteString(fmt.Sprintf("DTSTART:%s\r\n", dtstart))

		// End time - default to 1 hour if not set
		var endDateTime time.Time
		if !event.EndTime.IsZero() {
			endDateTime = event.EndTime
		} else {
			endDateTime = startDateTime.Add(1 * time.Hour)
		}
		dtend := endDateTime.Format("20060102T150405")
		sb.WriteString(fmt.Sprintf("DTEND:%s\r\n", dtend))
	}

	// Optional fields
	if event.Description != "" {
		sb.WriteString(fmt.Sprintf("DESCRIPTION:%s\r\n", escapeICSValue(event.Description)))
	}
	if event.Location != "" {
		sb.WriteString(fmt.Sprintf("LOCATION:%s\r\n", escapeICSValue(event.Location)))
	}

	// Free/Busy status
	if event.FreeBusy == StatusFree {
		sb.WriteString("TRANSP:TRANSPARENT\r\n")
	} else {
		sb.WriteString("TRANSP:OPAQUE\r\n")
	}

	sb.WriteString("STATUS:CONFIRMED\r\n")
	sb.WriteString("END:VEVENT\r\n")
	sb.WriteString("END:VCALENDAR\r\n")

	return sb.String()
}

// escapeICSValue escapes special characters for ICS format
func escapeICSValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, ";", "\\;")
	value = strings.ReplaceAll(value, ",", "\\,")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return value
}

// VdirCalendar represents a calendar detected from vdirsyncer
type VdirCalendar struct {
	Name        string
	DisplayName string
	Color       string
	Path        string
}

// detectVdirsyncerCalendars finds calendars from vdirsyncer storage
func detectVdirsyncerCalendars() ([]Calendar, []Event) {
	var calendars []Calendar
	var events []Event

	// Find vdirsyncer storage paths
	storagePaths := findVdirsyncerStoragePaths()
	if len(storagePaths) == 0 {
		return nil, nil
	}

	// Load color overrides from zen-cal config
	colorOverrides := loadCalendarColorOverrides()

	// Track seen calendars to avoid duplicates (by path and by name)
	seenPaths := make(map[string]bool)
	seenNames := make(map[string]bool)

	// Scan each storage path for calendars
	for _, storagePath := range storagePaths {
		vdirCals := scanVdirsyncerStorage(storagePath)
		for _, vdirCal := range vdirCals {
			// Skip if we've already seen this calendar path
			if seenPaths[vdirCal.Path] {
				continue
			}
			seenPaths[vdirCal.Path] = true

			// Skip if we've already seen this calendar name
			if seenNames[vdirCal.Name] {
				continue
			}
			seenNames[vdirCal.Name] = true

			// Check for color override in zen-cal config
			color := vdirCal.Color
			if override, ok := colorOverrides[vdirCal.Name]; ok {
				color = override
			}

			// If no color found, assign one based on index
			if color == "" {
				color = getDefaultColorForIndex(len(calendars))
			}

			calendar := Calendar{
				Name:        vdirCal.Name,
				DisplayName: vdirCal.DisplayName,
				Color:       lipgloss.Color(color),
			}
			calendars = append(calendars, calendar)

			// Load events from this calendar
			calEvents := loadEventsFromVdirCalendar(vdirCal.Path, vdirCal.Name)
			events = append(events, calEvents...)
		}
	}

	return calendars, events
}

// findVdirsyncerStoragePaths parses vdirsyncer config to find storage paths
func findVdirsyncerStoragePaths() []string {
	var paths []string
	seenPaths := make(map[string]bool)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return paths
	}

	// Helper to add path if not already seen
	addPath := func(path string) {
		// Resolve to absolute path for proper deduplication
		absPath, err := filepath.Abs(path)
		if err != nil {
			absPath = path
		}
		if !seenPaths[absPath] {
			if info, err := os.Stat(absPath); err == nil && info.IsDir() {
				seenPaths[absPath] = true
				paths = append(paths, absPath)
			}
		}
	}

	// Check common vdirsyncer config locations
	configPaths := []string{
		filepath.Join(homeDir, ".config", "vdirsyncer", "config"),
		filepath.Join(homeDir, ".vdirsyncer", "config"),
	}

	for _, configPath := range configPaths {
		foundPaths := parseVdirsyncerConfig(configPath)
		for _, p := range foundPaths {
			addPath(p)
		}
	}

	// Also check default storage locations even without config
	defaultPaths := []string{
		filepath.Join(homeDir, ".local", "share", "vdirsyncer"),
		filepath.Join(homeDir, ".calendars"),
		filepath.Join(homeDir, ".local", "share", "calendars"),
	}

	for _, defaultPath := range defaultPaths {
		addPath(defaultPath)
	}

	return paths
}

// parseVdirsyncerConfig extracts storage paths from vdirsyncer config
func parseVdirsyncerConfig(configPath string) []string {
	var paths []string

	file, err := os.Open(configPath)
	if err != nil {
		return paths
	}
	defer file.Close()

	// Regex to match path configurations in vdirsyncer config
	// Format: path = ~/.local/share/vdirsyncer/calendars/
	// or: path = /some/absolute/path
	pathRegex := regexp.MustCompile(`^\s*path\s*=\s*(.+?)\s*$`)
	homeDir, _ := os.UserHomeDir()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip comments
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		matches := pathRegex.FindStringSubmatch(line)
		if len(matches) == 2 {
			path := strings.Trim(matches[1], `"'`)

			// Expand ~ to home directory
			if strings.HasPrefix(path, "~/") {
				path = filepath.Join(homeDir, path[2:])
			} else if strings.HasPrefix(path, "~") {
				path = filepath.Join(homeDir, path[1:])
			}

			// Check if path exists
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				paths = append(paths, path)
			}
		}
	}

	return paths
}

// scanVdirsyncerStorage scans a storage directory for calendars
func scanVdirsyncerStorage(storagePath string) []VdirCalendar {
	var calendars []VdirCalendar

	entries, err := os.ReadDir(storagePath)
	if err != nil {
		return calendars
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		calPath := filepath.Join(storagePath, entry.Name())

		// Check if this directory contains .ics files (indicating it's a calendar)
		if !hasICSFiles(calPath) {
			// Check subdirectories (some setups have account/calendar structure)
			subEntries, err := os.ReadDir(calPath)
			if err != nil {
				continue
			}
			for _, subEntry := range subEntries {
				if subEntry.IsDir() {
					subCalPath := filepath.Join(calPath, subEntry.Name())
					if hasICSFiles(subCalPath) {
						cal := readVdirCalendar(subCalPath, subEntry.Name())
						calendars = append(calendars, cal)
					}
				}
			}
			continue
		}

		cal := readVdirCalendar(calPath, entry.Name())
		calendars = append(calendars, cal)
	}

	return calendars
}

// hasICSFiles checks if a directory contains any .ics files
func hasICSFiles(dirPath string) bool {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".ics") {
			return true
		}
	}
	return false
}

// readVdirCalendar reads calendar metadata from a vdir calendar directory
func readVdirCalendar(calPath string, defaultName string) VdirCalendar {
	cal := VdirCalendar{
		Name:        defaultName,
		DisplayName: defaultName,
		Path:        calPath,
	}

	// Try to read displayname
	displaynamePath := filepath.Join(calPath, "displayname")
	if data, err := os.ReadFile(displaynamePath); err == nil {
		displayName := strings.TrimSpace(string(data))
		if displayName != "" {
			cal.DisplayName = displayName
		}
	}

	// Try to read color
	colorPath := filepath.Join(calPath, "color")
	if data, err := os.ReadFile(colorPath); err == nil {
		color := strings.TrimSpace(string(data))
		// Normalize color format
		color = normalizeColor(color)
		if color != "" {
			cal.Color = color
		}
	}

	return cal
}

// normalizeColor ensures color is in #RRGGBB format
func normalizeColor(color string) string {
	color = strings.TrimSpace(color)

	// Remove any quotes
	color = strings.Trim(color, `"'`)

	// If it's already a valid hex color, return it
	hexRegex := regexp.MustCompile(`^#?([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	if hexRegex.MatchString(color) {
		if !strings.HasPrefix(color, "#") {
			color = "#" + color
		}
		return color
	}

	// Try to parse rgb() format
	rgbRegex := regexp.MustCompile(`rgb\s*\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)`)
	if matches := rgbRegex.FindStringSubmatch(color); len(matches) == 4 {
		// Convert to hex (simplified - would need proper parsing in production)
		return ""
	}

	return ""
}

// loadEventsFromVdirCalendar loads events from .ics files in a calendar directory
func loadEventsFromVdirCalendar(calPath string, calendarName string) []Event {
	var events []Event

	entries, err := os.ReadDir(calPath)
	if err != nil {
		return events
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".ics") {
			continue
		}

		icsPath := filepath.Join(calPath, entry.Name())
		icsEvents := parseICSFile(icsPath, calendarName)
		events = append(events, icsEvents...)
	}

	return events
}

// parseICSFile parses a single .ics file and extracts events
func parseICSFile(icsPath string, calendarName string) []Event {
	var events []Event

	file, err := os.Open(icsPath)
	if err != nil {
		return events
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	var currentEvent *Event
	var currentField string
	var currentValue strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// Handle line folding (lines starting with space or tab are continuations)
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if currentField != "" {
				currentValue.WriteString(strings.TrimLeft(line, " \t"))
			}
			continue
		}

		// Process the previous field if we were building one
		if currentEvent != nil && currentField != "" {
			processICSField(currentEvent, currentField, currentValue.String())
		}

		// Reset for new field
		currentField = ""
		currentValue.Reset()

		// Parse the line
		if strings.HasPrefix(line, "BEGIN:VEVENT") {
			currentEvent = &Event{
				CalendarName: calendarName,
				AllDay:       true,
				FreeBusy:     StatusBusy,
				FilePath:     icsPath,
			}
		} else if strings.HasPrefix(line, "END:VEVENT") {
			if currentEvent != nil && !currentEvent.Date.IsZero() && currentEvent.Title != "" {
				events = append(events, *currentEvent)
			}
			currentEvent = nil
		} else if currentEvent != nil {
			// Parse field:value or field;params:value
			colonIdx := strings.Index(line, ":")
			if colonIdx > 0 {
				fieldPart := line[:colonIdx]
				valuePart := line[colonIdx+1:]

				// Extract field name (before any semicolon parameters)
				semiIdx := strings.Index(fieldPart, ";")
				if semiIdx > 0 {
					currentField = strings.ToUpper(fieldPart[:semiIdx])
					// Check for VALUE=DATE parameter (indicates all-day event)
					params := strings.ToUpper(fieldPart[semiIdx:])
					if strings.Contains(params, "VALUE=DATE") {
						currentEvent.AllDay = true
					}
				} else {
					currentField = strings.ToUpper(fieldPart)
				}

				currentValue.WriteString(valuePart)
			}
		}
	}

	// Process any remaining field
	if currentEvent != nil && currentField != "" {
		processICSField(currentEvent, currentField, currentValue.String())
	}

	return events
}

// processICSField processes a single field from an ICS file
func processICSField(event *Event, field string, value string) {
	value = unescapeICSValue(value)

	switch field {
	case "SUMMARY":
		event.Title = value

	case "DESCRIPTION":
		event.Description = value

	case "LOCATION":
		event.Location = value

	case "UID":
		event.UID = value

	case "DTSTART":
		date, timeStr, allDay := parseICSDateTime(value)
		if !date.IsZero() {
			event.Date = date
			event.Time = timeStr
			event.AllDay = allDay
		}

	case "DTEND":
		endDate, _, _ := parseICSDateTime(value)
		if !endDate.IsZero() {
			event.EndTime = endDate
		}

	case "TRANSP":
		// TRANSPARENT means free, OPAQUE means busy
		if strings.ToUpper(value) == "TRANSPARENT" {
			event.FreeBusy = StatusFree
		} else {
			event.FreeBusy = StatusBusy
		}

	case "STATUS":
		// CANCELLED events should be marked differently (we'll skip them)
		if strings.ToUpper(value) == "CANCELLED" {
			event.Title = "" // This will cause the event to be skipped
		}
	}
}

// parseICSDateTime parses an ICS datetime value
func parseICSDateTime(value string) (time.Time, string, bool) {
	value = strings.TrimSpace(value)

	// Try various formats
	formats := []struct {
		format string
		allDay bool
		hasTime bool
	}{
		// Date only (all-day events)
		{"20060102", true, false},
		// Date with time (local)
		{"20060102T150405", false, true},
		// Date with time (UTC)
		{"20060102T150405Z", false, true},
	}

	for _, f := range formats {
		if t, err := time.Parse(f.format, value); err == nil {
			timeStr := ""
			if f.hasTime {
				timeStr = t.Format("15:04")
			}
			return t, timeStr, f.allDay
		}
	}

	return time.Time{}, "", true
}

// unescapeICSValue unescapes special characters in ICS values
func unescapeICSValue(value string) string {
	// ICS escapes: \n -> newline, \, -> comma, \; -> semicolon, \\ -> backslash
	value = strings.ReplaceAll(value, "\\n", "\n")
	value = strings.ReplaceAll(value, "\\N", "\n")
	value = strings.ReplaceAll(value, "\\,", ",")
	value = strings.ReplaceAll(value, "\\;", ";")
	value = strings.ReplaceAll(value, "\\\\", "\\")
	return value
}

// loadCalendarColorOverrides loads color overrides from zen-cal config
func loadCalendarColorOverrides() map[string]string {
	overrides := make(map[string]string)
	hexColor := regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return overrides
	}

	configPath := filepath.Join(homeDir, ".config", "zen-cal", "zen-cal.conf")
	file, err := os.Open(configPath)
	if err != nil {
		return overrides
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

			// Parse color (before optional display name)
			valParts := strings.SplitN(val, "|", 2)
			colorStr := strings.TrimSpace(valParts[0])

			if hexColor.MatchString(colorStr) {
				overrides[calName] = colorStr
			}
		}
	}

	return overrides
}

// getDefaultColorForIndex returns a color from a predefined palette based on index
func getDefaultColorForIndex(index int) string {
	colors := []string{
		"#f38ba8", // pink
		"#89b4fa", // blue
		"#a6e3a1", // green
		"#fab387", // orange
		"#cba6f7", // purple
		"#94e2d5", // teal
		"#f9e2af", // yellow
		"#eba0ac", // maroon
	}

	return colors[index%len(colors)]
}