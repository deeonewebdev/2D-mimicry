# 2D Mimicry – Quantile‑Based Fingerprint Generator

[![Go Reference](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**2D Mimicry** is a pair of command‑line tools that learn structural patterns from a list of strings (e.g., domain names, passwords, identifiers) and generate new strings that mimic the same statistical properties.  
It uses **position‑wise quantile fingerprints** and **bigram transition fingerprints** to control both character choices and local transitions – hence “2D”.

- `extractor` – reads a plain text file (one string per line) and produces several JSON statistics files.
- `generator` – consumes those statistics and prints generated strings (optionally with a TLD suffix) to standard output.

The tools are written in pure Go (no external dependencies) and support 2, 4, or 8 quantiles.

---

## ✨ Features

- **Position‑aware** character frequency analysis.
- **Transition‑aware** bigram quantile modelling.
- **Fingerprint pair** filtering – generate only strings whose regular *and* transition fingerprints satisfy a frequency threshold.
- Configurable quantile count (2, 4, 8).
- Deterministic output (given the same input and parameters).
- Fast, memory‑efficient generation using pre‑computed JSON files.

---

## 🚀 Installation

### Prerequisites
- Go 1.21 or later (older versions may work, but 1.21+ is recommended).

### Build from source
```bash
git clone https://github.com/yourusername/2d-mimicry.git
cd 2d-mimicry
go build -o extractor ./extractor-quartile-dynamic-fin-stats.go
go build -o generator ./generator-quartile-dynamic-fin-stats.go
```

(You can rename the binaries to `2d-extractor` and `2d-generator` if you prefer.)

---

## 📖 Usage

### 1. Extractor – learn from a wordlist

```bash
./extractor <input-file> [quantiles]
```

- `<input-file>` : plain text file with **one string per line** (no extra whitespace).
- `[quantiles]`   : optional, `2`, `4` or `8` (default `4`).

**Example:**
```bash
./extractor my-domains.txt 4
```

The extractor produces the following files in the **current directory** (names depend on string length `L` and quantile count):

| File pattern                                      | Content |
|---------------------------------------------------|---------|
| `character-frequency-<L>.json`                   | per‑position sorted character frequencies |
| `character-transitions-<L>.json`                 | most frequent next character for each previous character |
| `transition-quantiles-<L>.json`                  | for each previous character: quantile buckets (character strings) |
| `transition-quantiles-debug-<L>.json`            | same as above but with full frequency info |
| `fingerprint-frequencies-<L>.json`               | regular fingerprints (position quantile digits) sorted by count |
| `transition-fingerprints-<L>.json`               | transition fingerprints (digit string of length L-1) |
| `fingerprint-pairs-<L>.txt`                      | every input line represented as `regular:transition` |
| `fingerprint-pairs-stats-<L>.json`               | aggregated counts of the above pairs, sorted descending |

When `quantiles != 4`, the quantile number is inserted before the length, e.g.  
`fingerprint-frequencies-8-12.json`.

### 2. Generator – produce new strings

```bash
./generator <length> <threshold> <tld> [quantiles]
```

- `<length>`      : positive integer – length of the strings to generate (must match the extracted stats).
- `<threshold>`   : float between `0` and `1` – only fingerprints whose count is **≥ max_count * threshold** are used.
- `<tld>`         : suffix appended to every generated string (e.g. `.com`, can be empty `""`).
- `[quantiles]`   : optional, must match the quantile value used during extraction (default `4`).

**Example:**
```bash
./generator 12 0.05 .com 4
```

The generator reads the JSON files produced by the extractor and writes generated strings to **stdout** (one per line).  
If the `fingerprint-pairs-stats-<L>.json` file exists, the generator uses the **pair‑based mode** – which is faster and respects both fingerprints simultaneously.  
Otherwise it falls back to the classic cross‑product mode (using `fingerprint-frequencies` and `transition-fingerprints`).

---

## 🧠 How it works (briefly)

1. **Fingerprints**  
   For each position `i`, characters are sorted by frequency and split into `k` quantile groups (e.g. top 25%, next 25%, …).  
   The *regular fingerprint* of a string is a sequence of quantile digits (0..k-1).

2. **Transition fingerprints**  
   For each ordered pair of characters `(a,b)`, we look at all bigrams starting with `a`, sort them by frequency, and again split into `k` quantiles.  
   The *transition fingerprint* is the sequence of quantile digits for each adjacent pair `(s[i], s[i+1])`.

3. **Pair‑based generation**  
   The extractor builds all `(regular, transition)` fingerprint pairs from the input.  
   The generator filters these pairs by their occurrence count (using `threshold`) and expands each surviving pair into concrete strings by walking through the quantile buckets and applying transition quantile constraints.

4. **Output**  
   All possible strings that match the selected fingerprint pair(s) are generated recursively.  
   For length `L` this can be huge, but the fingerprints are chosen so that the number remains manageable (e.g. thousands to millions).

---

## 📂 Example workflow

```bash
# 1. Learn from a list of existing domain names (length 10)
./extractor existing-domains.txt 4

# 2. Generate new domains of length 10, keeping only the top 10% fingerprints
./generator 10 0.10 .com 4 > new-domains.txt
```

---

## ⚠️ Notes & limitations

- The extractor assumes **all lines have the same length**. If the input file contains lines of varying length, the tool will exit with an error.
- The generator expects the stats files for the exact length and quantile count you specify.
- For very long lengths (>20) or very low thresholds, the number of generated strings can become extremely large – consider using `head` or piping to a file.
- The tools use **recursive depth‑first generation**; there is no built‑in limit, so you may want to add one yourself if needed.

---

## 🤝 Contributing

Contributions are welcome!  
Please read [CONTRIBUTING.md](./CONTRIBUTING.md) before submitting pull requests.

---

## 📄 License

This project is licensed under the MIT License – see the [LICENSE](./LICENSE) file for details.

---

## 🙏 Acknowledgements

Inspired by statistical fingerprinting techniques used in **markov embeddings** and domain generation algorithms (DGAs).  
The quantile approach balances fidelity and generalisation.
```

## 💰 Donate

If you find this project useful, consider supporting its development:

**Bitcoin:** `bc1qxm6ase7av7462gwkr5u9ehpr5lmswrt4uct4v6`
