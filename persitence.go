package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const nameRecent = "recent_search.json" // File name for all recent searches

func getRecentSearches() ([]string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("cannot resolve path")
	}

	fileName := nameRecent
	base := filepath.Dir(file)
	dir := filepath.Join(base, "data") // Save in a new folder called 'data'
	fullPath := filepath.Join(dir, fileName)

	jsonString, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	recentSearches := []string{}
	if err := json.Unmarshal(jsonString, &recentSearches); err != nil {
		return nil, err
	}
	return recentSearches, err
}

func addRecentSearch(word string) ([]byte, error) {
	// Read recent searches
	existingSearches, err := getRecentSearches()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// Append new search
	existingSearches = append(existingSearches, word)

	// Encode to JSON
	jsonString, err := json.Marshal(existingSearches)
	if err != nil {
		return nil, err
	}

	// Get runtime filepath
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("cannot resolve path")
	}

	// Make directory if not already exists
	base := filepath.Dir(file)
	dir := filepath.Join(base, "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	// Write to file (replaces old content)
	fileName := nameRecent
	fullPath := filepath.Join(dir, fileName)
	if err := os.WriteFile(fullPath, jsonString, 0644); err != nil {
		return nil, err
	}

	return jsonString, nil
}
