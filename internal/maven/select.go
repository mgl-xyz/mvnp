package maven

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// PromptSelectPackages shows a numbered package list and returns selected coordinates.
func PromptSelectPackages(out io.Writer, in io.Reader, entries []PackageEntry) ([]string, error) {
	unique := UniquePackageCoordinates(entries)
	if len(unique) == 0 {
		return nil, fmt.Errorf("no packages to select")
	}

	fmt.Fprintln(out, "Packages:")
	fmt.Fprint(out, FormatPackageList(unique, true))
	fmt.Fprintln(out, "Select packages to upgrade:")
	fmt.Fprintln(out, "  numbers   1,3,5 or 1-3")
	fmt.Fprintln(out, "  empty     all packages")
	fmt.Fprintln(out, "  q         cancel")
	fmt.Fprint(out, "> ")

	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if strings.EqualFold(line, "q") || strings.EqualFold(line, "quit") {
		return nil, fmt.Errorf("selection cancelled")
	}
	if line == "" {
		coords := make([]string, 0, len(unique))
		for _, entry := range unique {
			coords = append(coords, entry.Coordinate)
		}
		return coords, nil
	}

	indices, err := parseSelection(line, len(unique))
	if err != nil {
		return nil, err
	}
	var coords []string
	seen := make(map[string]struct{})
	for _, index := range indices {
		coord := unique[index-1].Coordinate
		if _, ok := seen[coord]; ok {
			continue
		}
		seen[coord] = struct{}{}
		coords = append(coords, coord)
	}
	return coords, nil
}

func parseSelection(input string, max int) ([]int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}
	seen := make(map[int]struct{})
	var indices []int
	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			if start > end {
				start, end = end, start
			}
			for i := start; i <= end; i++ {
				if i < 1 || i > max {
					return nil, fmt.Errorf("selection out of range: %d", i)
				}
				if _, ok := seen[i]; ok {
					continue
				}
				seen[i] = struct{}{}
				indices = append(indices, i)
			}
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid selection %q", part)
		}
		if value < 1 || value > max {
			return nil, fmt.Errorf("selection out of range: %d", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		indices = append(indices, value)
	}
	sortInts(indices)
	return indices, nil
}

func sortInts(values []int) {
	for i := 1; i < len(values); i++ {
		j := i
		for j > 0 && values[j-1] > values[j] {
			values[j-1], values[j] = values[j], values[j-1]
			j--
		}
	}
}
