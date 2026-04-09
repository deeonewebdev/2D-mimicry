package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// CharFreq represents a character and its frequency at a given position.
type CharFreq struct {
	Char string `json:"char"`
	Freq int    `json:"freq"`
}

// SigCount holds a signature string and its occurrence count.
type SigCount struct {
	Signature string `json:"signature"`
	Count     int    `json:"count"`
}

// PairCount holds a fingerprint pair and its occurrence count.
type PairCount struct {
	Pair  string `json:"pair"`
	Count int    `json:"count"`
}

// TransitionQuantiles stores for each preceding character the quantiles
// of next characters (sorted by frequency descending) – characters only.
// The number of quantiles is variable (2, 4, or 8).
type TransitionQuantiles map[string][][]string

// TransitionQuantilesDebug stores the same quantiles but with full CharFreq objects.
type TransitionQuantilesDebug map[string][][]CharFreq

func main() {
	// Parse command line arguments
	quantiles := 4 // default
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <input-file> [quantiles]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  quantiles: optional 2, 4, or 8 (default 4)\n")
		fmt.Fprintf(os.Stderr, "Output files: character-frequency-<length>.json, fingerprint-frequencies-<length>.json,\n")
		fmt.Fprintf(os.Stderr, "  character-transitions-<length>.json, transition-quartiles-<length>.json,\n")
		fmt.Fprintf(os.Stderr, "  transition-quartiles-debug-<length>.json, transition-fingerprints-<length>.json,\n")
		fmt.Fprintf(os.Stderr, "  fingerprint-pairs-<length>.txt, fingerprint-pairs-<length>-stats.json\n")
		fmt.Fprintf(os.Stderr, "  (when quantiles != 4, the quantile number is inserted before <length>)\n")
		os.Exit(1)
	}
	inputFile := os.Args[1]
	if len(os.Args) == 3 {
		switch os.Args[2] {
		case "2":
			quantiles = 2
		case "4":
			quantiles = 4
		case "8":
			quantiles = 8
		default:
			fmt.Fprintf(os.Stderr, "Error: quantiles must be 2, 4, or 8, got %s\n", os.Args[2])
			os.Exit(1)
		}
	}

	// Open input file
	f, err := os.Open(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening input file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	// ---------- FIRST PASS: count frequencies per position and transitions ----------
	scanner := bufio.NewScanner(f)
	var positions []map[rune]int            // frequency maps per position (rune keys)
	transitions := make(map[rune]map[rune]int) // prev -> next -> count
	lineNum := 0

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r") // handle Windows line endings
		runes := []rune(line)

		if lineNum == 0 {
			if len(runes) == 0 {
				fmt.Fprintln(os.Stderr, "First line is empty, cannot determine line length")
				os.Exit(1)
			}
			// Initialize slice of maps
			positions = make([]map[rune]int, len(runes))
			for i := range positions {
				positions[i] = make(map[rune]int)
			}
		} else {
			// Verify consistent line length
			if len(runes) != len(positions) {
				fmt.Fprintf(os.Stderr, "Line %d has %d characters, expected %d\n",
					lineNum+1, len(runes), len(positions))
				os.Exit(1)
			}
		}

		// Count per-position frequencies
		for i, r := range runes {
			positions[i][r]++
		}

		// Count transitions (prev -> next) for all but the last character
		for i := 0; i < len(runes)-1; i++ {
			prev := runes[i]
			next := runes[i+1]
			if transitions[prev] == nil {
				transitions[prev] = make(map[rune]int)
			}
			transitions[prev][next]++
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input file: %v\n", err)
		os.Exit(1)
	}

	if lineNum == 0 {
		fmt.Fprintln(os.Stderr, "Input file is empty")
		os.Exit(1)
	}

	// ---------- Build sorted per‑position frequency lists and quantile maps ----------
	sortedPositions := make([][]CharFreq, len(positions))
	quantileMaps := make([]map[rune]int, len(positions)) // maps rune to quantile digit 0..quantiles-1

	for i, m := range positions {
		// Convert map to slice of CharFreq
		cf := make([]CharFreq, 0, len(m))
		for r, cnt := range m {
			cf = append(cf, CharFreq{Char: string(r), Freq: cnt})
		}
		// Sort: by Freq descending, then Char ascending
		sort.Slice(cf, func(a, b int) bool {
			if cf[a].Freq == cf[b].Freq {
				return cf[a].Char < cf[b].Char
			}
			return cf[a].Freq > cf[b].Freq
		})
		sortedPositions[i] = cf

		// Compute quantile boundaries for this position
		L := len(cf)
		partSize := L / quantiles
		rem := L % quantiles
		// sizes[i] = number of items in quantile i
		sizes := make([]int, quantiles)
		for q := 0; q < quantiles; q++ {
			sizes[q] = partSize
			if q < rem {
				sizes[q]++
			}
		}
		// Create quantile map
		qmap := make(map[rune]int, L)
		idx := 0
		for q := 0; q < quantiles; q++ {
			for j := 0; j < sizes[q]; j++ {
				r := []rune(cf[idx].Char)[0]
				qmap[r] = q
				idx++
			}
		}
		quantileMaps[i] = qmap
	}

	// ---------- Build transition quantiles and a lookup map ----------
	// transQuantiles stores the quantiles (as slices of strings) for each prev character.
	transQuantiles := make(TransitionQuantiles)
	// transQuantilesDebug stores the same quantiles but with full CharFreq objects.
	transQuantilesDebug := make(TransitionQuantilesDebug)
	// transLookup maps prev -> next -> quantile digit (0..quantiles-1)
	transLookup := make(map[rune]map[rune]int)

	for prev, nextCounts := range transitions {
		// Build slice of (next, count) pairs
		type pair struct {
			next  rune
			count int
		}
		pairs := make([]pair, 0, len(nextCounts))
		for nxt, cnt := range nextCounts {
			pairs = append(pairs, pair{next: nxt, count: cnt})
		}
		// Sort by count descending, then next character ascending
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].count == pairs[j].count {
				return pairs[i].next < pairs[j].next
			}
			return pairs[i].count > pairs[j].count
		})

		// Convert sorted pairs to a list of CharFreq (for debug) and a list of strings (for regular)
		charFreqList := make([]CharFreq, len(pairs))
		nextList := make([]string, len(pairs))
		for i, p := range pairs {
			charFreqList[i] = CharFreq{Char: string(p.next), Freq: p.count}
			nextList[i] = string(p.next)
		}

		// Split into quantiles contiguous groups
		L := len(nextList)
		partSize := L / quantiles
		rem := L % quantiles
		sizes := make([]int, quantiles)
		for q := 0; q < quantiles; q++ {
			sizes[q] = partSize
			if q < rem {
				sizes[q]++
			}
		}

		// Build quantile slices (regular and debug)
		quantilesStr := make([][]string, quantiles)
		quantilesDebug := make([][]CharFreq, quantiles)
		start := 0
		for q := 0; q < quantiles; q++ {
			end := start + sizes[q]
			quantilesStr[q] = nextList[start:end]
			quantilesDebug[q] = charFreqList[start:end]
			start = end
		}

		// Store in transQuantiles (key as string)
		transQuantiles[string(prev)] = quantilesStr
		transQuantilesDebug[string(prev)] = quantilesDebug

		// Build lookup map for this prev
		lookup := make(map[rune]int)
		for digit, q := range quantilesStr {
			for _, chStr := range q {
				r := []rune(chStr)[0]
				lookup[r] = digit
			}
		}
		transLookup[prev] = lookup
	}

	// ---------- Build transition map (most frequent next character for each prev) ----------
	transMap := make(map[rune]rune)
	for prev, nextCounts := range transitions {
		// Build slice of (next, count) pairs
		type pair struct {
			next  rune
			count int
		}
		pairs := make([]pair, 0, len(nextCounts))
		for nxt, cnt := range nextCounts {
			pairs = append(pairs, pair{next: nxt, count: cnt})
		}
		// Sort by count descending, then next character ascending
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].count == pairs[j].count {
				return pairs[i].next < pairs[j].next
			}
			return pairs[i].count > pairs[j].count
		})
		// Take the first (most frequent)
		if len(pairs) > 0 {
			transMap[prev] = pairs[0].next
		}
	}

	// Helper to generate filename for quantile-dependent outputs
	makeFilename := func(base string, length int) string {
		if quantiles == 4 {
			return fmt.Sprintf("%s-%d.json", base, length)
		}
		return fmt.Sprintf("%s-%d-%d.json", base, quantiles, length)
	}
	makeTextFilename := func(base string, length int) string {
		if quantiles == 4 {
			return fmt.Sprintf("%s-%d.txt", base, length)
		}
		return fmt.Sprintf("%s-%d-%d.txt", base, quantiles, length)
	}

	// ---------- Write character-frequency-{length}.json (or with quantile) ----------
	charFreqFile := makeFilename("character-frequency", len(positions))
	charFreqJSON, err := json.MarshalIndent(sortedPositions, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling character frequencies: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(charFreqFile, charFreqJSON, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", charFreqFile, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s\n", charFreqFile)

	// ---------- Write character-transitions-{length}.json (independent of quantiles) ----------
	strTransMap := make(map[string]string)
	for prev, next := range transMap {
		strTransMap[string(prev)] = string(next)
	}
	transFile := fmt.Sprintf("character-transitions-%d.json", len(positions))
	transJSON, err := json.MarshalIndent(strTransMap, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling transition map: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(transFile, transJSON, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", transFile, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s\n", transFile)

	// ---------- Write transition-quantiles-...json (characters only) ----------
	transQuantFile := makeFilename("transition-quantiles", len(positions))
	transQuantJSON, err := json.MarshalIndent(transQuantiles, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling transition quantiles: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(transQuantFile, transQuantJSON, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", transQuantFile, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s\n", transQuantFile)

	// ---------- Write transition-quantiles-debug-...json (with frequencies) ----------
	transQuantDebugFile := makeFilename("transition-quantiles-debug", len(positions))
	transQuantDebugJSON, err := json.MarshalIndent(transQuantilesDebug, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling transition quantiles debug: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(transQuantDebugFile, transQuantDebugJSON, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", transQuantDebugFile, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s\n", transQuantDebugFile)

	// ---------- SECOND PASS: compute structural signatures, transition fingerprints,
	//            fingerprint pairs, and write pairs to text file ----------
	// Rewind the file
	if _, err := f.Seek(0, 0); err != nil {
		fmt.Fprintf(os.Stderr, "Error seeking to beginning: %v\n", err)
		os.Exit(1)
	}
	scanner = bufio.NewScanner(f)

	// Prepare output files for fingerprint pairs (text) and stats (JSON)
	pairsFile := makeTextFilename("fingerprint-pairs", len(positions))
	pairWriter, err := os.Create(pairsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", pairsFile, err)
		os.Exit(1)
	}
	defer pairWriter.Close()

	signatureCounts := make(map[string]int)
	transSigCounts := make(map[string]int)
	pairCounts := make(map[string]int)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		runes := []rune(line)

		// Compute regular signature (quantile-based)
		var sb strings.Builder
		for i, r := range runes {
			quantile, ok := quantileMaps[i][r]
			if !ok {
				fmt.Fprintf(os.Stderr, "Internal error: character %q at position %d not found in frequency map\n", string(r), i)
				os.Exit(1)
			}
			sb.WriteByte(byte('0' + quantile))
		}
		signature := sb.String()
		signatureCounts[signature]++

		// Compute transition signature (string of digits 0..quantiles-1 of length len-1)
		var tsb strings.Builder
		for i := 0; i < len(runes)-1; i++ {
			prev := runes[i]
			next := runes[i+1]
			lookup, ok := transLookup[prev]
			if !ok {
				// No transition info for this prev (should not happen if prev appears with next)
				tsb.WriteByte('0')
			} else {
				digit, ok := lookup[next]
				if !ok {
					// Next character not found in prev's list (should not happen)
					tsb.WriteByte('0')
				} else {
					tsb.WriteByte(byte('0' + digit))
				}
			}
		}
		transSig := tsb.String()
		if len(transSig) > 0 { // only if line length >= 2
			transSigCounts[transSig]++
		}

		// Build fingerprint pair: signature:transSig
		pair := signature + ":" + transSig
		pairCounts[pair]++

		// Write pair to text file (one per line)
		_, err := pairWriter.WriteString(pair + "\n")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to %s: %v\n", pairsFile, err)
			os.Exit(1)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error during second pass: %v\n", err)
		os.Exit(1)
	}

	// ---------- Build sorted list of regular signature counts ----------
	sigList := make([]SigCount, 0, len(signatureCounts))
	for sig, cnt := range signatureCounts {
		sigList = append(sigList, SigCount{Signature: sig, Count: cnt})
	}
	// Sort by Count descending, then Signature ascending
	sort.Slice(sigList, func(i, j int) bool {
		if sigList[i].Count == sigList[j].Count {
			return sigList[i].Signature < sigList[j].Signature
		}
		return sigList[i].Count > sigList[j].Count
	})

	// ---------- Write fingerprint-frequencies-...json ----------
	fingerprintFile := makeFilename("fingerprint-frequencies", len(positions))
	fingerprintJSON, err := json.MarshalIndent(sigList, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling fingerprint frequencies: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(fingerprintFile, fingerprintJSON, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", fingerprintFile, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s\n", fingerprintFile)

	// ---------- Build sorted list of transition signature counts ----------
	transSigList := make([]SigCount, 0, len(transSigCounts))
	for sig, cnt := range transSigCounts {
		transSigList = append(transSigList, SigCount{Signature: sig, Count: cnt})
	}
	// Sort by Count descending, then Signature ascending
	sort.Slice(transSigList, func(i, j int) bool {
		if transSigList[i].Count == transSigList[j].Count {
			return transSigList[i].Signature < transSigList[j].Signature
		}
		return transSigList[i].Count > transSigList[j].Count
	})

	// ---------- Write transition-fingerprints-...json ----------
	transFingerFile := makeFilename("transition-fingerprints", len(positions))
	transFingerJSON, err := json.MarshalIndent(transSigList, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling transition fingerprints: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(transFingerFile, transFingerJSON, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", transFingerFile, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s\n", transFingerFile)

	// ---------- Build and write fingerprint pairs statistics ----------
	pairStats := make([]PairCount, 0, len(pairCounts))
	for pair, cnt := range pairCounts {
		pairStats = append(pairStats, PairCount{Pair: pair, Count: cnt})
	}
	// Sort by Count descending, then Pair ascending
	sort.Slice(pairStats, func(i, j int) bool {
		if pairStats[i].Count == pairStats[j].Count {
			return pairStats[i].Pair < pairStats[j].Pair
		}
		return pairStats[i].Count > pairStats[j].Count
	})

	statsFile := makeFilename("fingerprint-pairs-stats", len(positions))
	statsJSON, err := json.MarshalIndent(pairStats, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling fingerprint pair statistics: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(statsFile, statsJSON, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", statsFile, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s\n", statsFile)
	fmt.Printf("Wrote %s\n", pairsFile)
}
