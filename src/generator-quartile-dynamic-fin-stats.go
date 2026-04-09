package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Fingerprint represents one entry in the fingerprint frequencies file.
type Fingerprint struct {
	Signature string `json:"signature"`
	Count     int    `json:"count"`
}

// PairStat represents one entry in the fingerprint-pairs-stats JSON.
type PairStat struct {
	Pair  string `json:"pair"`
	Count int    `json:"count"`
}

// CharFreq represents one character entry in the character frequency file.
type CharFreq struct {
	Char string `json:"char"`
	Freq int    `json:"freq"`
}

// TransitionQuantiles maps a preceding character (as string) to its quantiles
// of next characters (each quantile is a slice of strings). The number of
// quantiles is variable (2, 4, or 8).
type TransitionQuantiles map[string][][]string

func main() {
	// Check command line arguments
	// Usage: program <length> <threshold> <tld> [quantiles]
	// quantiles optional, default 4
	minArgs := 4
	maxArgs := 5
	if len(os.Args) < minArgs || len(os.Args) > maxArgs {
		fmt.Fprintf(os.Stderr, "Usage: %s <length> <threshold> <tld> [quantiles]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  length: positive integer\n")
		fmt.Fprintf(os.Stderr, "  threshold: float between 0 and 1\n")
		fmt.Fprintf(os.Stderr, "  tld: string to append (e.g., .com)\n")
		fmt.Fprintf(os.Stderr, "  quantiles: optional 2, 4, or 8 (default 4)\n")
		os.Exit(1)
	}

	length, err := strconv.Atoi(os.Args[1])
	if err != nil || length <= 0 {
		fmt.Fprintf(os.Stderr, "Invalid length: must be a positive integer\n")
		os.Exit(1)
	}

	threshold, err := strconv.ParseFloat(os.Args[2], 64)
	if err != nil || threshold < 0 || threshold > 1 {
		fmt.Fprintf(os.Stderr, "Invalid threshold: must be a float between 0 and 1\n")
		os.Exit(1)
	}

	tld := os.Args[3]

	quantiles := 4 // default
	if len(os.Args) == 5 {
		q, err := strconv.Atoi(os.Args[4])
		if err != nil || (q != 2 && q != 4 && q != 8) {
			fmt.Fprintf(os.Stderr, "Invalid quantiles: must be 2, 4, or 8\n")
			os.Exit(1)
		}
		quantiles = q
	}

	// Helper to build filenames depending on quantile count
	makeFilename := func(base string, length int) string {
		if quantiles == 4 {
			return fmt.Sprintf("%s-%d.json", base, length)
		}
		return fmt.Sprintf("%s-%d-%d.json", base, quantiles, length)
	}

	// Construct filenames
	charFile := makeFilename("character-frequency", length)
	transQuantFile := makeFilename("transition-quantiles", length)
	pairStatsFile := makeFilename("fingerprint-pairs-stats", length) // new

	// ----- Load character frequencies -----
	charData, err := os.ReadFile(charFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", charFile, err)
		os.Exit(1)
	}
	var allPositions [][]CharFreq
	if err := json.Unmarshal(charData, &allPositions); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", charFile, err)
		os.Exit(1)
	}
	if len(allPositions) != length {
		fmt.Fprintf(os.Stderr, "Mismatch: %s has %d positions, expected %d\n", charFile, len(allPositions), length)
		os.Exit(1)
	}

	// ----- Load transition quantiles -----
	transQuantData, err := os.ReadFile(transQuantFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", transQuantFile, err)
		os.Exit(1)
	}
	var transQuantiles TransitionQuantiles
	if err := json.Unmarshal(transQuantData, &transQuantiles); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", transQuantFile, err)
		os.Exit(1)
	}

	// ----- Decide which generation mode to use -----
	// Try to load fingerprint-pairs-stats file
	pairStatsData, err := os.ReadFile(pairStatsFile)
	if err == nil {
		// Mode 1: Use precomputed fingerprint pairs
		var pairStats []PairStat
		if err := json.Unmarshal(pairStatsData, &pairStats); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v, falling back to old mode\n", pairStatsFile, err)
			goto oldMode
		}
		if len(pairStats) == 0 {
			fmt.Fprintf(os.Stderr, "No fingerprint pairs found in %s, falling back to old mode\n", pairStatsFile)
			goto oldMode
		}

		// Compute threshold on pair counts
		maxCount := pairStats[0].Count
		minCount := int(float64(maxCount) * threshold)

		// Iterate over pairs that meet the threshold
		for _, ps := range pairStats {
			if ps.Count < minCount {
				break
			}
			// Split the pair into regular signature and transition signature
			parts := strings.SplitN(ps.Pair, ":", 2)
			if len(parts) != 2 {
				fmt.Fprintf(os.Stderr, "Warning: malformed pair %q, skipping\n", ps.Pair)
				continue
			}
			regSig := parts[0]
			transSig := parts[1]

			// Validate lengths
			if len(regSig) != length {
				fmt.Fprintf(os.Stderr, "Warning: regular signature %q length %d != %d, skipping\n", regSig, len(regSig), length)
				continue
			}
			if length > 1 && len(transSig) != length-1 {
				fmt.Fprintf(os.Stderr, "Warning: transition signature %q length %d != %d, skipping\n", transSig, len(transSig), length-1)
				continue
			}
			if length == 1 && transSig != "" {
				fmt.Fprintf(os.Stderr, "Warning: length=1 but transition signature not empty: %q, skipping\n", transSig)
				continue
			}

			// Validate digits in regular signature
			validReg := true
			for _, ch := range regSig {
				d := int(ch - '0')
				if d < 0 || d >= quantiles {
					fmt.Fprintf(os.Stderr, "Warning: regular signature %q contains digit %d out of range 0..%d, skipping\n", regSig, d, quantiles-1)
					validReg = false
					break
				}
			}
			if !validReg {
				continue
			}

			// Build positional character lists from regular signature
			positionChars := make([][]string, length)
			for i := 0; i < length; i++ {
				digit := int(regSig[i] - '0')
				sorted := allPositions[i]
				partsQuant := splitIntoQuantiles(sorted, quantiles)
				part := partsQuant[digit]
				chars := make([]string, len(part))
				for j, cf := range part {
					chars[j] = cf.Char
				}
				positionChars[i] = chars
			}

			// If length == 1, no transition constraints
			if length == 1 {
				generateCombinationsNoTrans(positionChars, 0, "", tld)
				continue
			}

			// Validate transition signature digits
			transDigits := make([]int, length-1)
			validTrans := true
			for i, ch := range transSig {
				d := int(ch - '0')
				if d < 0 || d >= quantiles {
					fmt.Fprintf(os.Stderr, "Warning: transition signature %q contains digit %d out of range 0..%d, skipping\n", transSig, d, quantiles-1)
					validTrans = false
					break
				}
				transDigits[i] = d
			}
			if !validTrans {
				continue
			}

			// Generate combinations with both constraints
			generateCombinationsWithTrans(positionChars, transQuantiles, transDigits, 0, "", tld, quantiles)
		}
		return // done, no fallback needed
	} else {
		fmt.Fprintf(os.Stderr, "Note: %s not found, falling back to old mode\n", pairStatsFile)
	}

oldMode:
	// ----- Old mode: cross product of regular and transition fingerprints -----
	fingerFile := makeFilename("fingerprint-frequencies", length)
	transFingerFile := makeFilename("transition-fingerprints", length)

	// Load fingerprint frequencies
	fingerData, err := os.ReadFile(fingerFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", fingerFile, err)
		os.Exit(1)
	}
	var fingerprints []Fingerprint
	if err := json.Unmarshal(fingerData, &fingerprints); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", fingerFile, err)
		os.Exit(1)
	}
	if len(fingerprints) == 0 {
		fmt.Fprintf(os.Stderr, "No fingerprints found in %s\n", fingerFile)
		os.Exit(1)
	}

	// Load transition fingerprints
	transFingerData, err := os.ReadFile(transFingerFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", transFingerFile, err)
		os.Exit(1)
	}
	var transFingerprints []Fingerprint
	if err := json.Unmarshal(transFingerData, &transFingerprints); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", transFingerFile, err)
		os.Exit(1)
	}

	// Compute thresholds
	maxCount := fingerprints[0].Count
	minCount := int(float64(maxCount) * threshold)
	var maxTransCount int
	if len(transFingerprints) > 0 {
		maxTransCount = transFingerprints[0].Count
	} else {
		maxTransCount = 0
	}
	minTransCount := int(float64(maxTransCount) * threshold)

	// Filter transition fingerprints
	selectedTrans := make([]string, 0)
	for _, tf := range transFingerprints {
		if tf.Count < minTransCount {
			break
		}
		if len(tf.Signature) != length-1 {
			fmt.Fprintf(os.Stderr, "Warning: transition fingerprint %q length %d != %d, skipping\n", tf.Signature, len(tf.Signature), length-1)
			continue
		}
		valid := true
		for _, ch := range tf.Signature {
			d := int(ch - '0')
			if d < 0 || d >= quantiles {
				fmt.Fprintf(os.Stderr, "Warning: transition fingerprint %q contains digit %d out of range 0..%d, skipping\n", tf.Signature, d, quantiles-1)
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		selectedTrans = append(selectedTrans, tf.Signature)
	}
	noTrans := (len(selectedTrans) == 0)

	// Process each regular fingerprint
	for _, fp := range fingerprints {
		if fp.Count < minCount {
			break
		}
		regSig := fp.Signature
		if len(regSig) != length {
			fmt.Fprintf(os.Stderr, "Warning: regular signature %q length %d != %d, skipping\n", regSig, len(regSig), length)
			continue
		}
		validReg := true
		for _, ch := range regSig {
			d := int(ch - '0')
			if d < 0 || d >= quantiles {
				fmt.Fprintf(os.Stderr, "Warning: regular signature %q contains digit %d out of range 0..%d, skipping\n", regSig, d, quantiles-1)
				validReg = false
				break
			}
		}
		if !validReg {
			continue
		}

		positionChars := make([][]string, length)
		for i := 0; i < length; i++ {
			digit := int(regSig[i] - '0')
			sorted := allPositions[i]
			parts := splitIntoQuantiles(sorted, quantiles)
			part := parts[digit]
			chars := make([]string, len(part))
			for j, cf := range part {
				chars[j] = cf.Char
			}
			positionChars[i] = chars
		}

		if noTrans {
			generateCombinationsNoTrans(positionChars, 0, "", tld)
			continue
		}

		for _, transSig := range selectedTrans {
			digits := make([]int, length-1)
			validTrans := true
			for i, ch := range transSig {
				d := int(ch - '0')
				if d < 0 || d >= quantiles {
					fmt.Fprintf(os.Stderr, "Warning: invalid digit %c in transition signature %q, skipping\n", ch, transSig)
					validTrans = false
					break
				}
				digits[i] = d
			}
			if !validTrans {
				continue
			}
			generateCombinationsWithTrans(positionChars, transQuantiles, digits, 0, "", tld, quantiles)
		}
	}
}

// splitIntoQuantiles divides a sorted slice of CharFreq into quantiles contiguous parts
// as evenly as possible. Returns a slice of slices, each containing the CharFreq objects.
func splitIntoQuantiles(list []CharFreq, quantiles int) [][]CharFreq {
	L := len(list)
	partSize := L / quantiles
	rem := L % quantiles

	sizes := make([]int, quantiles)
	for q := 0; q < quantiles; q++ {
		sizes[q] = partSize
		if q < rem {
			sizes[q]++
		}
	}

	parts := make([][]CharFreq, quantiles)
	start := 0
	for q := 0; q < quantiles; q++ {
		end := start + sizes[q]
		parts[q] = list[start:end]
		start = end
	}
	return parts
}

// generateCombinationsNoTrans recursively builds all strings from the given lists
// and prints each completed string appended with the TLD.
func generateCombinationsNoTrans(lists [][]string, pos int, current string, tld string) {
	if pos == len(lists) {
		fmt.Println(current + tld)
		return
	}
	for _, ch := range lists[pos] {
		generateCombinationsNoTrans(lists, pos+1, current+ch, tld)
	}
}

// generateCombinationsWithTrans recursively builds strings satisfying both
// positional and transition quantile constraints.
func generateCombinationsWithTrans(lists [][]string, transQuantiles TransitionQuantiles, digits []int, pos int, current string, tld string, quantiles int) {
	if pos == len(lists) {
		fmt.Println(current + tld)
		return
	}
	if pos == 0 {
		for _, ch := range lists[0] {
			generateCombinationsWithTrans(lists, transQuantiles, digits, pos+1, current+ch, tld, quantiles)
		}
		return
	}
	prevChar := string(current[pos-1])
	quantilesForPrev, ok := transQuantiles[prevChar]
	if !ok {
		return
	}
	transDigit := digits[pos-1]
	if transDigit < 0 || transDigit >= len(quantilesForPrev) {
		return
	}
	allowedTransSet := make(map[string]bool)
	for _, ch := range quantilesForPrev[transDigit] {
		allowedTransSet[ch] = true
	}
	for _, ch := range lists[pos] {
		if allowedTransSet[ch] {
			generateCombinationsWithTrans(lists, transQuantiles, digits, pos+1, current+ch, tld, quantiles)
		}
	}
}
