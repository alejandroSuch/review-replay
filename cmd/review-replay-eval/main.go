// review-replay-eval: capture/run/diff fixtures for accuracy and stability
// measurement.
//
// This binary lives next to the main one and shares the package internals via
// the same module. It mirrors the TypeScript eval scripts: capture per-PR
// fixtures preserving labels, run the classifier over labelled fixtures with
// a confusion-matrix output, and diff two runs to surface flips.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alejandroSuch/review-replay/internal/classifier"
	"github.com/alejandroSuch/review-replay/internal/evidence"
	"github.com/alejandroSuch/review-replay/internal/github"
	"github.com/alejandroSuch/review-replay/internal/llm"
	"github.com/alejandroSuch/review-replay/internal/types"
)

const usage = `review-replay-eval <command> [args]

Commands:
  capture <pr> [out-dir]            Write one fixture per top-level comment.
                                    Preserves existing labels.
  run [fixtures-dir] [--save NAME]  Classify labelled fixtures, report
                                    accuracy + confusion matrix.
  diff <run-a> <run-b>              Compare two saved runs (flips, churn).
  capture-batch <prs.txt> [out]     Capture many PRs from a list file.
  discover [--org X] [--max N] [--min-reviews K]
                                    Search an org for PRs with review activity,
                                    write the URLs to eval/corpus/prs.txt.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "capture":
		err = runCapture(args)
	case "run":
		err = runEval(args)
	case "diff":
		err = runDiff(args)
	case "capture-batch":
		err = runCaptureBatch(args)
	case "discover":
		err = runDiscover(args)
	case "-h", "--help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n%s", cmd, usage)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// Fixture is the on-disk shape; tagged kind discriminates inline vs issue-level.
type Fixture struct {
	Kind     string          `json:"kind"` // "inline" | "issue-level"
	Source   FixtureSource   `json:"source"`
	Label    Label           `json:"label"`
	Evidence json.RawMessage `json:"evidence"`
}

type FixtureSource struct {
	PR        string `json:"pr"`
	CommentID int64  `json:"commentId"`
}

// Label is intentionally single-annotator for now: one status plus free-form
// notes. This is too lossy for ambiguous fixtures (partial vs needs-discussion,
// addressed vs needs-discussion on borderline cases) and will need a richer
// schema when more than one annotator labels the same fixtures. Tracked at
// https://github.com/alejandroSuch/review-replay/issues/1.
type Label struct {
	Status types.ClassificationStatus `json:"status,omitempty"`
	Notes  string                     `json:"notes"`
}

func runCapture(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: capture <pr> [out-dir]")
	}
	prArg := args[0]
	outDir := "eval/fixtures"
	if len(args) >= 2 {
		outDir = args[1]
	}
	pr, err := github.ParsePrRef(prArg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	gh, err := github.NewClient(ctx)
	if err != nil {
		return err
	}
	snap, err := gh.FetchPrSnapshot(ctx, pr)
	if err != nil {
		return err
	}
	builder := evidence.New(gh)
	inline, issueLevel, err := builder.BuildAll(ctx, snap)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	slug := fmt.Sprintf("%s-%s-%d", pr.Owner, pr.Repo, pr.Number)
	prRef := fmt.Sprintf("%s/%s#%d", pr.Owner, pr.Repo, pr.Number)

	var written, refreshed int
	for _, e := range inline {
		path := filepath.Join(outDir, fmt.Sprintf("%s-%d.json", slug, e.Comment.ID))
		if err := writeFixture(path, "inline", FixtureSource{PR: prRef, CommentID: e.Comment.ID}, e, &written, &refreshed); err != nil {
			return err
		}
	}
	for _, e := range issueLevel {
		tag := "summary"
		if e.Comment.Kind == types.KindIssueComment {
			tag = "issue"
		}
		path := filepath.Join(outDir, fmt.Sprintf("%s-%s-%d.json", slug, tag, e.Comment.ID))
		if err := writeFixture(path, "issue-level", FixtureSource{PR: prRef, CommentID: e.Comment.ID}, e, &written, &refreshed); err != nil {
			return err
		}
	}
	fmt.Printf("Captured %d new fixtures, refreshed %d existing (labels preserved) under %s\n", written, refreshed, outDir)
	return nil
}

func writeFixture(path, kind string, src FixtureSource, evidence any, written, refreshed *int) error {
	existing, existedLabel := readExistingLabel(path)
	if existing {
		*refreshed++
	} else {
		*written++
	}
	rawEv, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	fx := Fixture{
		Kind:     kind,
		Source:   src,
		Label:    existedLabel,
		Evidence: rawEv,
	}
	out, err := json.MarshalIndent(fx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

func readExistingLabel(path string) (bool, Label) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, Label{}
	}
	var prev Fixture
	if err := json.Unmarshal(data, &prev); err != nil {
		return false, Label{}
	}
	return true, prev.Label
}

func runEval(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var saveName string
	var inlineOnly, issueOnly bool
	fs.StringVar(&saveName, "save", "", "save run under eval/runs/<name>.json")
	fs.BoolVar(&inlineOnly, "inline-only", false, "")
	fs.BoolVar(&issueOnly, "issue-level-only", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	dir := "eval/fixtures"
	if len(rest) >= 1 {
		dir = rest[0]
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type loaded struct {
		id   string
		path string
		fx   Fixture
	}
	all := []loaded{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		var fx Fixture
		if err := json.Unmarshal(raw, &fx); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if inlineOnly && fx.Kind != "inline" {
			continue
		}
		if issueOnly && fx.Kind != "issue-level" {
			continue
		}
		all = append(all, loaded{
			id:   strings.TrimSuffix(e.Name(), ".json"),
			path: e.Name(),
			fx:   fx,
		})
	}

	labelled := all[:0]
	for _, l := range all {
		if l.fx.Label.Status != "" {
			labelled = append(labelled, l)
		}
	}
	if len(labelled) == 0 {
		return fmt.Errorf("%d fixtures found, none labelled", len(all))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	provider, resolved, err := llm.Resolve(llm.Config{})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Running %d labelled fixtures with %s/%s\n\n", len(labelled), resolved.Provider, resolved.Model)

	type result struct {
		ID         string                     `json:"id"`
		Kind       string                     `json:"kind"`
		Expected   types.ClassificationStatus `json:"expected"`
		Predicted  types.ClassificationStatus `json:"predicted"`
		Source     types.ClassificationSource `json:"source"`
		RuleName   string                     `json:"ruleName,omitempty"`
		Confidence float64                    `json:"confidence"`
		Rationale  string                     `json:"rationale"`
		Notes      string                     `json:"notes"`
		Match      bool                       `json:"match"`
		RawText    string                     `json:"rawText,omitempty"`
		ParseError string                     `json:"parseError,omitempty"`
	}
	results := make([]result, 0, len(labelled))
	for _, l := range labelled {
		opts := classifier.Options{LLM: provider, Model: resolved.Model}
		var (
			cls    types.Classification
			source types.ClassificationSource
			rule   string
			raw    string
			perr   string
		)
		if l.fx.Kind == "inline" {
			var e types.CommentEvidence
			if err := json.Unmarshal(l.fx.Evidence, &e); err != nil {
				return fmt.Errorf("%s: decode evidence: %w", l.id, err)
			}
			res, err := classifier.ClassifyComment(ctx, e, opts)
			if err != nil {
				return err
			}
			cls = res.Classification
			source = res.Diagnostics.Source
			rule = res.Diagnostics.RuleName
			raw = res.Diagnostics.RawText
			perr = res.Diagnostics.ParseError
		} else {
			var e types.IssueLevelEvidence
			if err := json.Unmarshal(l.fx.Evidence, &e); err != nil {
				return fmt.Errorf("%s: decode evidence: %w", l.id, err)
			}
			res, err := classifier.ClassifyIssueLevel(ctx, e, opts)
			if err != nil {
				return err
			}
			cls = res.Classification
			source = res.Diagnostics.Source
			rule = res.Diagnostics.RuleName
			raw = res.Diagnostics.RawText
			perr = res.Diagnostics.ParseError
		}
		match := cls.Status == l.fx.Label.Status
		results = append(results, result{
			ID:         l.id,
			Kind:       l.fx.Kind,
			Expected:   l.fx.Label.Status,
			Predicted:  cls.Status,
			Source:     source,
			RuleName:   rule,
			Confidence: cls.Confidence,
			Rationale:  cls.Rationale,
			Notes:      l.fx.Label.Notes,
			Match:      match,
			RawText:    raw,
			ParseError: perr,
		})
		if match {
			fmt.Fprint(os.Stderr, ".")
		} else {
			fmt.Fprint(os.Stderr, "x")
		}
	}
	fmt.Fprint(os.Stderr, "\n\n")

	correct := 0
	for _, r := range results {
		if r.Match {
			correct++
		}
	}
	total := len(results)
	fmt.Printf("accuracy: %.1f%% (%d/%d)\n\n", float64(correct)/float64(total)*100, correct, total)

	// Per kind + per source.
	perKind := map[string]struct{ c, t int }{}
	perSource := map[string]struct{ c, t int }{}
	for _, r := range results {
		k := perKind[r.Kind]
		k.t++
		if r.Match {
			k.c++
		}
		perKind[r.Kind] = k
		s := perSource[string(r.Source)]
		s.t++
		if r.Match {
			s.c++
		}
		perSource[string(r.Source)] = s
	}
	for kind, v := range perKind {
		fmt.Printf("  by kind  %-14s: %.1f%% (%d/%d)\n", kind, float64(v.c)/float64(v.t)*100, v.c, v.t)
	}
	for src, v := range perSource {
		fmt.Printf("  by src   %-14s: %.1f%% (%d/%d)\n", src, float64(v.c)/float64(v.t)*100, v.c, v.t)
	}

	statuses := []types.ClassificationStatus{
		types.StatusAddressed, types.StatusPartial, types.StatusPending, types.StatusNeedsDiscussion,
	}

	// Per-class precision / recall / F1.
	type pr struct {
		tp, fp, fn int
	}
	stats := map[types.ClassificationStatus]*pr{}
	for _, s := range statuses {
		stats[s] = &pr{}
	}
	for _, r := range results {
		if r.Expected == r.Predicted {
			stats[r.Expected].tp++
		} else {
			stats[r.Predicted].fp++
			stats[r.Expected].fn++
		}
	}
	fmt.Println("\nper-class precision / recall / F1:")
	fmt.Printf("  %-18s %9s %9s %9s\n", "class", "prec", "recall", "f1")
	for _, s := range statuses {
		p := safeDiv(float64(stats[s].tp), float64(stats[s].tp+stats[s].fp))
		rc := safeDiv(float64(stats[s].tp), float64(stats[s].tp+stats[s].fn))
		f1 := 0.0
		if p+rc > 0 {
			f1 = 2 * p * rc / (p + rc)
		}
		fmt.Printf("  %-18s %9.2f %9.2f %9.2f\n", s, p, rc, f1)
	}

	// False-addressed rate: the product cares about this more than any other
	// failure mode. Of all non-addressed ground truths, how many got
	// predicted as addressed?
	var falseAddressed, nonAddressedTotal int
	for _, r := range results {
		if r.Expected == types.StatusAddressed {
			continue
		}
		nonAddressedTotal++
		if r.Predicted == types.StatusAddressed {
			falseAddressed++
		}
	}
	fmt.Println("\nfalse-addressed rate (predicted addressed when ground truth was not):")
	if nonAddressedTotal == 0 {
		fmt.Println("  n/a (no non-addressed fixtures)")
	} else {
		fmt.Printf("  %d / %d = %.1f%%\n", falseAddressed, nonAddressedTotal, float64(falseAddressed)/float64(nonAddressedTotal)*100)
	}

	// Confidence calibration: bucket predictions by confidence band and report
	// accuracy in each band. If conf >= 0.85 doesn't beat conf < 0.65 by a
	// wide margin, the model isn't actually telling you when to trust it.
	type band struct {
		label string
		lo    float64
		hi    float64
	}
	bands := []band{
		{">= 0.85", 0.85, 1.01},
		{"0.65 - 0.85", 0.65, 0.85},
		{"< 0.65", 0.0, 0.65},
	}
	fmt.Println("\nconfidence calibration:")
	fmt.Printf("  %-12s %9s %9s\n", "band", "acc", "n")
	for _, b := range bands {
		correct, total := 0, 0
		for _, r := range results {
			if r.Confidence < b.lo || r.Confidence >= b.hi {
				continue
			}
			total++
			if r.Match {
				correct++
			}
		}
		acc := safeDiv(float64(correct), float64(total)) * 100
		fmt.Printf("  %-12s %8.1f%% %9d\n", b.label, acc, total)
	}

	fmt.Println("\nconfusion matrix (rows=expected, cols=predicted):")
	header := strings.Builder{}
	header.WriteString(strings.Repeat(" ", 18))
	for _, s := range statuses {
		header.WriteString(fmt.Sprintf(" %9s", trunc(string(s), 9)))
	}
	fmt.Println(header.String())
	for _, expected := range statuses {
		row := strings.Builder{}
		row.WriteString(fmt.Sprintf("%18s", expected))
		for _, predicted := range statuses {
			count := 0
			for _, r := range results {
				if r.Expected == expected && r.Predicted == predicted {
					count++
				}
			}
			row.WriteString(fmt.Sprintf(" %9d", count))
		}
		fmt.Println(row.String())
	}

	misses := []result{}
	for _, r := range results {
		if !r.Match {
			misses = append(misses, r)
		}
	}
	if len(misses) > 0 {
		fmt.Println("\nmisses:")
		for _, m := range misses {
			tag := ""
			if m.ParseError != "" {
				tag = " [PARSE-FAIL]"
			}
			fmt.Printf("  [%s] %s expected=%s got=%s conf=%.2f%s\n", m.Kind, m.ID, m.Expected, m.Predicted, m.Confidence, tag)
			fmt.Printf("    rationale: %s\n", m.Rationale)
			notes := m.Notes
			if notes == "" {
				notes = "(none)"
			}
			fmt.Printf("    notes:     %s\n", notes)
			if m.ParseError != "" {
				fmt.Printf("    parseError: %s\n", m.ParseError)
			}
		}
	}

	if saveName != "" {
		perClass := map[string]map[string]float64{}
		for _, s := range statuses {
			p := safeDiv(float64(stats[s].tp), float64(stats[s].tp+stats[s].fp))
			rc := safeDiv(float64(stats[s].tp), float64(stats[s].tp+stats[s].fn))
			f1 := 0.0
			if p+rc > 0 {
				f1 = 2 * p * rc / (p + rc)
			}
			perClass[string(s)] = map[string]float64{
				"precision": p,
				"recall":    rc,
				"f1":        f1,
				"tp":        float64(stats[s].tp),
				"fp":        float64(stats[s].fp),
				"fn":        float64(stats[s].fn),
			}
		}
		var falseAddressedRate float64
		if nonAddressedTotal > 0 {
			falseAddressedRate = float64(falseAddressed) / float64(nonAddressedTotal)
		}
		calibration := map[string]map[string]float64{}
		for _, b := range bands {
			cb, tb := 0, 0
			for _, r := range results {
				if r.Confidence < b.lo || r.Confidence >= b.hi {
					continue
				}
				tb++
				if r.Match {
					cb++
				}
			}
			calibration[b.label] = map[string]float64{
				"accuracy": safeDiv(float64(cb), float64(tb)),
				"correct":  float64(cb),
				"total":    float64(tb),
			}
		}
		saved := map[string]any{
			"runAt":                 time.Now().UTC().Format(time.RFC3339),
			"provider":              resolved.Provider,
			"model":                 resolved.Model,
			"fixtures":              total,
			"labelled":              total,
			"accuracy":              float64(correct) / float64(total),
			"perKind":               perKindToMap(perKind),
			"perSource":             perKindToMap(perSource),
			"perClass":              perClass,
			"falseAddressedRate":    falseAddressedRate,
			"falseAddressedCount":   falseAddressed,
			"nonAddressedTotal":     nonAddressedTotal,
			"confidenceCalibration": calibration,
			"results":               results,
		}
		if err := os.MkdirAll("eval/runs", 0o755); err != nil {
			return err
		}
		path := filepath.Join("eval/runs", saveName+".json")
		out, err := json.MarshalIndent(saved, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("\nsaved: %s\n", path)
	}
	return nil
}

func perKindToMap(in map[string]struct{ c, t int }) map[string]map[string]int {
	out := make(map[string]map[string]int, len(in))
	for k, v := range in {
		out[k] = map[string]int{"correct": v.c, "total": v.t}
	}
	return out
}

func runDiff(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: diff <run-a> <run-b>")
	}
	a, err := loadRun(args[0])
	if err != nil {
		return err
	}
	b, err := loadRun(args[1])
	if err != nil {
		return err
	}
	aByID := map[string]map[string]any{}
	for _, r := range a.Results {
		aByID[r["id"].(string)] = r
	}
	bByID := map[string]map[string]any{}
	for _, r := range b.Results {
		bByID[r["id"].(string)] = r
	}
	common := []string{}
	for id := range aByID {
		if _, ok := bByID[id]; ok {
			common = append(common, id)
		}
	}

	var fixes, regressions, churn int
	flips := []struct {
		id, aStatus, bStatus, expected string
		aConf, bConf                   float64
		bRationale                     string
	}{}
	for _, id := range common {
		ar, br := aByID[id], bByID[id]
		if ar["predicted"] == br["predicted"] {
			continue
		}
		aMatch := ar["match"].(bool)
		bMatch := br["match"].(bool)
		switch {
		case !aMatch && bMatch:
			fixes++
		case aMatch && !bMatch:
			regressions++
		default:
			churn++
		}
		flips = append(flips, struct {
			id, aStatus, bStatus, expected string
			aConf, bConf                   float64
			bRationale                     string
		}{
			id:         id,
			aStatus:    ar["predicted"].(string),
			bStatus:    br["predicted"].(string),
			expected:   br["expected"].(string),
			aConf:      ar["confidence"].(float64),
			bConf:      br["confidence"].(float64),
			bRationale: br["rationale"].(string),
		})
	}

	fmt.Printf("A: %s/%s @ %s → %.1f%%\n", a.Provider, a.Model, a.RunAt, a.Accuracy*100)
	fmt.Printf("B: %s/%s @ %s → %.1f%%\n", b.Provider, b.Model, b.RunAt, b.Accuracy*100)
	fmt.Printf("Δ accuracy: %+.1fpp\n\n", (b.Accuracy-a.Accuracy)*100)
	fmt.Printf("Fixes: %d · Regressions: %d · Churn: %d\n", fixes, regressions, churn)

	// False-addressed rate — the product-critical risk metric.
	fmt.Printf("\nΔ false-addressed rate: A=%.1f%%  B=%.1f%%  (%+.1fpp)\n",
		a.FalseAddressedRate*100, b.FalseAddressedRate*100, (b.FalseAddressedRate-a.FalseAddressedRate)*100)

	// Per-class precision / recall / F1 deltas.
	if len(a.PerClass) > 0 && len(b.PerClass) > 0 {
		statusOrder := []string{"addressed", "partial", "pending", "needs-discussion"}
		fmt.Println("\nΔ per-class (B - A):")
		fmt.Printf("  %-18s %10s %10s %10s\n", "class", "Δprec", "Δrecall", "Δf1")
		for _, s := range statusOrder {
			am, ok := a.PerClass[s]
			bm, ok2 := b.PerClass[s]
			if !ok || !ok2 {
				continue
			}
			fmt.Printf("  %-18s %+10.2f %+10.2f %+10.2f\n",
				s,
				bm["precision"]-am["precision"],
				bm["recall"]-am["recall"],
				bm["f1"]-am["f1"],
			)
		}
	}

	// Confidence calibration deltas.
	if len(a.ConfidenceCalibration) > 0 && len(b.ConfidenceCalibration) > 0 {
		bands := []string{">= 0.85", "0.65 - 0.85", "< 0.65"}
		fmt.Println("\nΔ calibration accuracy (B - A):")
		fmt.Printf("  %-12s %10s %10s %10s\n", "band", "A acc", "B acc", "Δ")
		for _, label := range bands {
			am, ok := a.ConfidenceCalibration[label]
			bm, ok2 := b.ConfidenceCalibration[label]
			if !ok || !ok2 {
				continue
			}
			fmt.Printf("  %-12s %9.1f%% %9.1f%% %+9.1fpp\n",
				label, am["accuracy"]*100, bm["accuracy"]*100, (bm["accuracy"]-am["accuracy"])*100)
		}
	}

	if len(flips) > 0 {
		fmt.Println("\nFlipped predictions:")
		for _, f := range flips {
			fmt.Printf("  %s\n", f.id)
			fmt.Printf("    A→B: %s → %s (expected %s)\n", f.aStatus, f.bStatus, f.expected)
			fmt.Printf("    rationale: %s\n", f.bRationale)
		}
	}
	return nil
}

type savedRun struct {
	RunAt                 string                                `json:"runAt"`
	Provider              string                                `json:"provider"`
	Model                 string                                `json:"model"`
	Accuracy              float64                               `json:"accuracy"`
	PerClass              map[string]map[string]float64         `json:"perClass"`
	FalseAddressedRate    float64                               `json:"falseAddressedRate"`
	FalseAddressedCount   int                                   `json:"falseAddressedCount"`
	NonAddressedTotal     int                                   `json:"nonAddressedTotal"`
	ConfidenceCalibration map[string]map[string]float64         `json:"confidenceCalibration"`
	Results               []map[string]any                      `json:"results"`
}

func loadRun(name string) (*savedRun, error) {
	path := name
	if !strings.HasSuffix(name, ".json") {
		path = filepath.Join("eval/runs", name+".json")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s savedRun
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func runCaptureBatch(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: capture-batch <prs.txt> [out-dir]")
	}
	listFile := args[0]
	outDir := "eval/corpus/fixtures"
	if len(args) >= 2 {
		outDir = args[1]
	}
	raw, err := os.ReadFile(listFile)
	if err != nil {
		return err
	}
	lines := strings.Split(string(raw), "\n")
	urls := lines[:0]
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			urls = append(urls, l)
		}
	}
	fmt.Fprintf(os.Stderr, "Capturing %d PRs into %s\n", len(urls), outDir)
	ok, failed := 0, 0
	for i, url := range urls {
		fmt.Fprintf(os.Stderr, "[%d/%d] %s\n", i+1, len(urls), url)
		if err := runCapture([]string{url, outDir}); err != nil {
			fmt.Fprintln(os.Stderr, "  failed:", err)
			failed++
			continue
		}
		ok++
	}
	fmt.Fprintf(os.Stderr, "\nCaptured: %d ok, %d failed.\n", ok, failed)
	if failed > 0 {
		return errors.New("some captures failed")
	}
	return nil
}

func runDiscover(args []string) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	var org string
	var max, minReviews int
	var outFile string
	var state string
	fs.StringVar(&org, "org", "gitkraken", "")
	fs.IntVar(&max, "max", 60, "")
	fs.IntVar(&minReviews, "min-reviews", 3, "")
	fs.StringVar(&outFile, "out", "eval/corpus/prs.txt", "")
	fs.StringVar(&state, "state", "merged", "merged | closed | open")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()
	gh, err := github.NewClient(ctx)
	if err != nil {
		return err
	}
	_ = gh // we use the gh CLI for the search since it's already authenticated
	type searchedPr struct {
		URL        string `json:"url"`
		Repository struct {
			NameWithOwner string `json:"nameWithOwner"`
		} `json:"repository"`
		Number int `json:"number"`
	}
	ghArgs := []string{"search", "prs", "--owner", org, "--limit", "100", "--json", "url,repository,number"}
	switch state {
	case "merged":
		ghArgs = append(ghArgs, "--merged", "--state", "closed")
	case "closed":
		ghArgs = append(ghArgs, "--state", "closed")
	case "open":
		ghArgs = append(ghArgs, "--state", "open")
	}
	cmd := exec.CommandContext(ctx, "gh", ghArgs...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("gh search failed: %w", err)
	}
	var pool []searchedPr
	if err := json.Unmarshal(out, &pool); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Found %d candidates. Filtering by review-comment count >= %d...\n", len(pool), minReviews)

	kept := []string{}
	for i, p := range pool {
		if len(kept) >= max {
			break
		}
		// count via gh api per-PR (cheaper than rolling our own auth here)
		c := exec.CommandContext(ctx, "gh", "api", fmt.Sprintf("/repos/%s/pulls/%d/comments", p.Repository.NameWithOwner, p.Number), "--paginate", "-q", "length")
		raw, err := c.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s#%d: %v\n", p.Repository.NameWithOwner, p.Number, err)
			continue
		}
		total := 0
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			var n int
			fmt.Sscanf(strings.TrimSpace(line), "%d", &n)
			total += n
		}
		if total >= minReviews {
			fmt.Fprintf(os.Stderr, "  ✓ %s#%d (%d review comments)\n", p.Repository.NameWithOwner, p.Number, total)
			kept = append(kept, p.URL)
		}
		if (i+1)%10 == 0 {
			fmt.Fprintf(os.Stderr, "  [%d/%d]\n", i+1, len(pool))
		}
	}
	if err := os.MkdirAll(filepath.Dir(outFile), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outFile, []byte(strings.Join(kept, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\nWrote %d PRs to %s\n", len(kept), outFile)
	return nil
}

func safeDiv(num, den float64) float64 {
	if den == 0 {
		return 0
	}
	return num / den
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
