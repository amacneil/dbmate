package dbutil

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// DefaultTimestampLayout specifies previous default timestamp with YYYYMMDDtime format
const DefaultTimestampLayout = "20060102150405"

// NewTimestampLayout specifies timestamp with YYYY_MM_DD_time format
const NewTimestampLayout = "2006_01_02_150405"

// DefaultTimestampSeparator specifies new timestamp separator
const DefaultTimestampSeparator = "_"

// TimeSlice holds a slice of File for the purpose of sorting
type TimeSlice []File

// File stores information regarding a migration file
type File struct {
	Timestamp         time.Time
	OriginalTimestamp string
	Name              string
	Description       string
}

func (p TimeSlice) Len() int {
	return len(p)
}

func (p TimeSlice) Less(i, j int) bool {
	return p[i].Timestamp.Before(p[j].Timestamp)
}

func (p TimeSlice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// SortByDate picks the timestamp part of the file whether it is using the old
// or new format, and sort them chronologically.
func SortByDate(matches []string) (sorted []string, err error) {
	timestamps := make(TimeSlice, 0, len(sorted))

	for _, match := range matches {
		timestamp, description, found := Cut(match, DefaultTimestampSeparator)
		if !found {
			return nil, errors.New("unable to find timestamp")
		}
		var parsed time.Time
		if len(timestamp) == len(DefaultTimestampLayout) {
			// Handles previous timestamp format which is 14 characters long
			p, err := time.Parse(DefaultTimestampLayout, timestamp)
			if err != nil {
				return nil, err
			}
			parsed = p
		} else if len(timestamp) == len(NewTimestampLayout) {
			// Handles new timestamp format which is 17 characters long
			p, err := time.Parse(NewTimestampLayout, timestamp)
			if err != nil {
				return nil, err
			}
			parsed = p
		}

		timestamps = append(timestamps, File{
			Timestamp:         parsed,
			OriginalTimestamp: timestamp,
			Description:       description,
			Name:              match,
		})
	}

	// Len(), Less(), and Swap() are custom implemented and satisfy
	// sort.Interface
	sort.Sort(timestamps)

	for _, timestamp := range timestamps {
		sorted = append(sorted, timestamp.Name)
	}

	return sorted, nil
}

func Cut(val string, separator string) (timestamp, description string, found bool) {
	split := strings.Split(val, separator)
	if len(split) > 1 {
		if len(split[0]) == len(DefaultTimestampLayout) {
			timestamp = split[0]
			description = strings.Join(split[1:], "_")
		} else if len(split[0]) == 4 { // equal to YYYY
			timestamp = split[0] + DefaultTimestampSeparator +
				split[1] + DefaultTimestampSeparator +
				split[2] + DefaultTimestampSeparator +
				split[3]
			description = strings.Join(split[4:], "_")
		} else {
			return "", "", false
		}
	} else {
		return "", "", false
	}

	return timestamp, description, true
}
