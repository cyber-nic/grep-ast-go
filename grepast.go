package grepast

import (
	"fmt"
	"regexp"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TreeContext stores context about source code lines, parsing, scopes, and line-of-interest management.
type TreeContext struct {
	filename                 string             // Name of the file being processed.
	source                   []byte             // Source code content as a byte array.
	color                    bool               // Whether to use color for highlighted output.
	verbose                  bool               // Whether to enable verbose output for debugging.
	lineNumber               bool               // Whether to include line numbers in the output.
	lastLine                 bool               // Whether to always include the last line in the output.
	margin                   int                // Number of lines to include as a margin at the top of the output.
	markLOIs                 bool               // Whether to visually mark lines of interest (LOI).
	headerMax                int                // Maximum number of header lines to display.
	loiPad                   int                // Number of lines of padding around lines of interest.
	showTopOfFileParentScope bool               // Whether to include the parent scope starting from the top of the file.
	parentContext            bool               // Whether to include parent context in the output.
	childContext             bool               // Whether to include child context in the output.
	lines                    []string           // Source code split into individual lines.
	numLines                 int                // Total number of lines in the source code (including an optional trailing newline adjustment).
	outputLines              map[int]string     // Map of output lines, optionally with highlights.
	scopes                   []map[int]struct{} // Tracks scope relationships by line.
	header                   [][]int            // Each element is a slice representing [startLine, endLine] of headers.
	nodes                    [][]*sitter.Node   // Tracks parse-tree nodes indexed by their start line.
	showLines                map[int]struct{}   // Lines to show in the final output.
	linesOfInterest          map[int]struct{}   // Lines explicitly marked as "lines of interest" (LOI).
	doneParentScopes         map[int]struct{}   // Tracks parent scopes that have already been processed.
}

// TreeContextOptions specifies various options for initializing TreeContext.
type TreeContextOptions struct {
	Color                    bool // Use colored output for matches or highlights.
	Verbose                  bool // Enable verbose mode for additional debugging or insights.
	ShowLineNumber           bool // Include line numbers in the output.
	ShowParentContext        bool // Show the parent scope of lines of interest in the output.
	ShowChildContext         bool // Show the child scope of lines of interest in the output.
	ShowLastLine             bool // Always include the last line in the output.
	MarginPadding            int  // Number of lines to add as a margin at the top of the output.
	MarkLinesOfInterest      bool // Visually mark lines of interest (LOI) in the output.
	HeaderMax                int  // Maximum number of header lines to display.
	ShowTopOfFileParentScope bool // Always include the top-most parent scope from the file's beginning.
	LinesOfInterestPadding   int  // Number of lines of padding around each line of interest.
}

// NewTreeContext is the Go-equivalent constructor for TreeContext.
// It initializes the context for analyzing and working with source code.
func NewTreeContext(filename string, source []byte, options TreeContextOptions) (*TreeContext, error) {
	// Get the language from the filename.
	// Determines the programming language to use for parsing based on the file extension.
	lang, _, err := GetLanguageFromFileName(filename)
	if err != nil {
		return nil, err // Return an error if the file type cannot be recognized.
	}

	// Return an error if the language is not supported.
	if lang == nil {
		return nil, fmt.Errorf("unrecognized or unsupported file type (%s)", filename)
	}

	// Initialize Tree-sitter parser for parsing source code into an abstract syntax tree (AST).
	parser := sitter.NewParser()
	parser.SetLanguage(lang) // Set the parser's language to match the file type.

	// Parse the source code into a syntax tree.
	tree := parser.Parse(source, nil)

	// Retrieve the root node of the syntax tree for traversal.
	rootNode := tree.RootNode()

	// Split the source code into lines for easier processing.
	lines := strings.Split(string(source), "\n")
	numLines := len(lines)
	if len(source) > 0 && source[len(source)-1] == '\n' {
		// Adjust for a trailing newline, aligning with Python's len+1 logic.
		numLines += 0
	}

	// Initialize scopes, headers, and nodes for tracking relationships and parsing metadata.
	scopes := make([]map[int]struct{}, numLines+1) // +1 to mimic Python’s len+1 logic.
	header := make([][]int, numLines+1)            // Track start and end lines for each header.
	nodes := make([][]*sitter.Node, numLines+1)    // Track AST nodes by their starting line.
	for i := 0; i <= numLines; i++ {
		scopes[i] = make(map[int]struct{})
		header[i] = []int{0, 0}
		nodes[i] = []*sitter.Node{}
	}

	// Create and populate the TreeContext object with initialized values.
	tc := &TreeContext{
		filename:                 filename,
		source:                   source,
		color:                    options.Color,
		verbose:                  options.Verbose,
		lineNumber:               options.ShowLineNumber,
		parentContext:            options.ShowParentContext,
		childContext:             options.ShowChildContext,
		lastLine:                 options.ShowLastLine,
		margin:                   options.MarginPadding,
		markLOIs:                 options.MarkLinesOfInterest,
		headerMax:                options.HeaderMax,
		loiPad:                   options.LinesOfInterestPadding,
		showTopOfFileParentScope: options.ShowTopOfFileParentScope,
		lines:                    lines,
		numLines:                 numLines + 1, // Account for potential trailing newlines.
		outputLines:              make(map[int]string),
		scopes:                   scopes,
		header:                   header,
		nodes:                    nodes,
		showLines:                make(map[int]struct{}),
		linesOfInterest:          make(map[int]struct{}),
		doneParentScopes:         make(map[int]struct{}),
	}

	// Walk through the parse tree to populate headers, scopes, and nodes.
	tc.walkTree(rootNode, 0)

	// Perform additional processing on scopes and headers after tree traversal.
	tc.postWalkProcessing()

	// Return the initialized TreeContext object.
	return tc, nil
}

// postWalkProcessing sets header ranges and optionally prints scopes.
func (tc *TreeContext) postWalkProcessing() {
	// print and set header ranges
	var scopeWidth int

	if tc.verbose {
		// find the maximum width for printing scopes
		for i := 0; i < tc.numLines-1; i++ {
			scopeStr := fmt.Sprintf("%v", mapKeysSorted(tc.scopes[i]))
			if len(scopeStr) > scopeWidth {
				scopeWidth = len(scopeStr)
			}
		}
	}

	for i := 0; i < tc.numLines; i++ {
		headerSlice := tc.header[i]
		if len(headerSlice) < 2 {
			// default
			tc.header[i] = []int{i, i + 1}
		} else {
			size := headerSlice[0]
			headStart := headerSlice[1]
			headEnd := headerSlice[1] + 1
			if len(headerSlice) > 2 {
				headEnd = headerSlice[2]
			}
			if size > tc.headerMax {
				headEnd = headStart + tc.headerMax
			}
			tc.header[i] = []int{headStart, headEnd}
		}

		if tc.verbose && i < tc.numLines-1 {
			scopeStr := fmt.Sprintf("%v", mapKeysSorted(tc.scopes[i]))
			if i < len(tc.lines) {
				lineStr := tc.lines[i]
				fmt.Printf("%-*s %3d %s\n", scopeWidth, scopeStr, i, lineStr)
			}
		}
	}
}

// Grep finds lines matching a pattern and highlights them.
func (tc *TreeContext) Grep(pat string, ignoreCase bool) map[int]struct{} {
	found := make(map[int]struct{})
	if ignoreCase {
		// Go's regex doesn't have "IGNORECASE" as a flag (like Python),
		// you compile different patterns or use (?i).
		pat = "(?i)" + pat
	}
	re := regexp.MustCompile(pat)

	for i, line := range tc.lines {
		if re.FindStringIndex(line) != nil {
			// highlight
			if tc.color {
				highlighted := re.ReplaceAllStringFunc(line, func(m string) string {
					return fmt.Sprintf("\033[1;31m%s\033[0m", m)
				})
				tc.outputLines[i] = highlighted
			}
			found[i] = struct{}{}
		}
	}
	return found
}

// AddLinesOfInterest adds lines of interest.
func (tc *TreeContext) AddLinesOfInterest(lineNums map[int]struct{}) {
	for ln := range lineNums {
		tc.linesOfInterest[ln] = struct{}{}
	}
}

// AddContext expands lines to show (showLines) based on linesOfInterest.
func (tc *TreeContext) AddContext() {
	if len(tc.linesOfInterest) == 0 {
		return
	}

	// Ensure all linesOfInterest are in showLines
	for line := range tc.linesOfInterest {
		tc.showLines[line] = struct{}{}
	}

	// Add padding lines around each LOI
	if tc.loiPad > 0 {
		var toAdd []int
		for line := range tc.showLines {
			start := line - tc.loiPad
			end := line + tc.loiPad
			for nl := start; nl <= end; nl++ {
				if nl < 0 || nl >= tc.numLines {
					continue
				}
				toAdd = append(toAdd, nl)
			}
		}
		for _, x := range toAdd {
			tc.showLines[x] = struct{}{}
		}
	}

	// Optionally add bottom line (plus parent context)
	if tc.lastLine {
		bottomLine := tc.numLines - 2
		tc.showLines[bottomLine] = struct{}{}
		tc.addParentScopes(bottomLine)
	}

	// Add parent contexts
	if tc.parentContext {
		for i := range tc.linesOfInterest {
			tc.addParentScopes(i)
		}
	}

	// Add child contexts
	// NOTE: This is where we fix partial expansions. If you want the entire function body,
	// you can remove or adjust the logic in addChildContext.
	if tc.childContext {
		for i := range tc.linesOfInterest {
			tc.addChildContext(i)
		}
	}

	// Add top margin lines
	if tc.margin > 0 {
		for i := 0; i < tc.margin && i < tc.numLines; i++ {
			tc.showLines[i] = struct{}{}
		}
	}

	// Close small gaps between lines to produce a smoother snippet
	tc.closeSmallGaps()
}

// addChildContext tries to show a child scope for the line i (e.g. function body),
// replicating the Python logic more closely.  If the scope is small (<5 lines),
// we reveal everything.  Otherwise, we show partial expansions by calling
// addParentScopes(childStart) for each child, up to a max limit.
func (tc *TreeContext) addChildContext(i int) {
	if i < 0 || i >= len(tc.nodes) {
		return
	}
	if len(tc.nodes[i]) == 0 {
		return
	}

	lastLine := tc.getLastLineOfScope(i)
	size := lastLine - i
	if size < 0 {
		return
	}

	// If the scope is small enough, reveal everything.
	if size < 5 {
		for line := i; line <= lastLine && line < tc.numLines; line++ {
			tc.showLines[line] = struct{}{}
		}
		return
	}

	// Gather all children for node(s) on line i, then sort by size descending.
	children := []*sitter.Node{}
	for _, node := range tc.nodes[i] {
		children = append(children, tc.findAllChildren(node)...)
	}
	sortNodesBySize(children)

	currentlyShowing := len(tc.showLines)

	// We only reveal ~10% of the larger scope, at least 5 lines, at most 25 lines,
	// matching the Python logic.
	maxToShow := 25
	minToShow := 5
	percentToShow := 0.10
	computedMax := int(float64(size)*percentToShow + 0.5)
	if computedMax < minToShow {
		computedMax = minToShow
	} else if computedMax > maxToShow {
		computedMax = maxToShow
	}

	// For each child, we only expand up to computedMax times by revealing
	// its parent scopes.  (Mirrors Python's "self.add_parent_scopes(child_start_line)")
	for _, child := range children {
		if len(tc.showLines) > currentlyShowing+computedMax {
			break
		}
		childStart := int(child.StartPosition().Row)
		// childEnd := int(child.EndPosition().Row)
		// for line := childStart; line <= childEnd && line < tc.numLines; line++ {
		// 	tc.showLines[line] = struct{}{}
		// }
		tc.addParentScopes(childStart)
	}
}

// findAllChildren gathers all descendants (recursive)
func (tc *TreeContext) findAllChildren(node *sitter.Node) []*sitter.Node {
	out := []*sitter.Node{node}
	for i := uint(0); i < node.ChildCount(); i++ {
		if child := node.NamedChild(i); child != nil {
			out = append(out, tc.findAllChildren(child)...)
		}
	}
	return out
}

// getLastLineOfScope finds the maximum end_line for nodes that start on line i
func (tc *TreeContext) getLastLineOfScope(i int) int {
	if i < 0 || i >= len(tc.nodes) || len(tc.nodes[i]) == 0 {
		return i
	}
	lastLine := 0
	for _, node := range tc.nodes[i] {
		if int(node.EndPosition().Row) > lastLine {
			lastLine = int(node.EndPosition().Row)
		}
	}
	return lastLine
}

// closeSmallGaps closes single-line gaps.
func (tc *TreeContext) closeSmallGaps() {
	closedShow := make(map[int]struct{}, len(tc.showLines))
	for k := range tc.showLines {
		closedShow[k] = struct{}{}
	}

	sortedShow := mapKeysSorted(tc.showLines)

	// fill i+1 if i and i+2 are present
	for i := 0; i < len(sortedShow)-1; i++ {
		curr := sortedShow[i]
		next := sortedShow[i+1]
		if next-curr == 2 {
			closedShow[curr+1] = struct{}{}
		}
	}

	// pick up adjacent blank lines
	for i, line := range tc.lines {
		if _, ok := closedShow[i]; ok {
			if strings.TrimSpace(line) != "" && i < tc.numLines-2 {
				// check if next line is blank
				if len(tc.lines) > i+1 && strings.TrimSpace(tc.lines[i+1]) == "" {
					closedShow[i+1] = struct{}{}
				}
			}
		}
	}

	tc.showLines = closedShow
}

// Format outputs the final lines. This version prints an initial ellipsis
// if the first line is NOT in showLines, replicating the Python code's
// "dots = not (0 in self.show_lines)" behavior.
func (tc *TreeContext) Format() string {
	if len(tc.showLines) == 0 {
		return ""
	}

	var sb strings.Builder

	// Optional color reset at the start
	if tc.color {
		sb.WriteString("\033[0m\n")
	}

	// If the first line is *not* in showLines, we begin in "ellipses" mode,
	// so we will print an ellipsis when we next skip lines.
	_, firstLineShown := tc.showLines[0]
	printEllipsis := !firstLineShown

	for i, line := range tc.lines {
		_, shouldShow := tc.showLines[i]
		if !shouldShow {
			// Print ellipsis once after last shown line
			if printEllipsis {
				sb.WriteString("⋮...\n")
				printEllipsis = false
			}
			continue
		}

		// Show the line
		spacer := tc.lineOfInterestSpacer(i)
		oline := tc.highlightedOrOriginalLine(i, line)
		if tc.lineNumber {
			fmt.Fprintf(&sb, "%3d%s%s\n", i+1, spacer, oline)
		} else {
			fmt.Fprintf(&sb, "%s%s\n", spacer, oline)
		}

		// If we skip lines after this, we want an ellipsis
		printEllipsis = true
	}

	return sb.String()
}

// lineOfInterestSpacer returns "│" or "█" (with color if needed)
func (tc *TreeContext) lineOfInterestSpacer(i int) string {
	if _, isLOI := tc.linesOfInterest[i]; isLOI && tc.markLOIs {
		if tc.color {
			return "\033[31m█\033[0m"
		}
		return "█"
	}
	return "│"
}

// highlightedOrOriginalLine uses the highlighted version if present
func (tc *TreeContext) highlightedOrOriginalLine(i int, original string) string {
	if hl, ok := tc.outputLines[i]; ok {
		return hl
	}
	return original
}

// addParentScopes recursively shows lines for parent scopes
func (tc *TreeContext) addParentScopes(i int) {
	if i < 0 || i >= len(tc.scopes) {
		return
	}
	if _, done := tc.doneParentScopes[i]; done {
		return
	}
	tc.doneParentScopes[i] = struct{}{}

	// for each scope that starts at line_num
	for lineNum := range tc.scopes[i] {
		headerSlice := tc.header[lineNum]
		if len(headerSlice) >= 2 {
			headStart := headerSlice[0]
			headEnd := headerSlice[1]
			if headStart > 0 || tc.showTopOfFileParentScope {
				for ln := headStart; ln < headEnd && ln < tc.numLines; ln++ {
					tc.showLines[ln] = struct{}{}
				}
			}
			// optionally add last line
			if tc.lastLine {
				lastLine := tc.getLastLineOfScope(lineNum)
				tc.addParentScopes(lastLine)
			}
		}
	}
}

// walkTree populates scopes, headers, etc.
func (tc *TreeContext) walkTree(node *sitter.Node, depth int) (int, int) {
	startLine := int(node.StartPosition().Row)
	endLine := int(node.EndPosition().Row)
	size := endLine - startLine

	if startLine < 0 || startLine >= len(tc.nodes) {
		return startLine, endLine
	}
	tc.nodes[startLine] = append(tc.nodes[startLine], node)

	// if tc.verbose && node.IsNamed() {
	// 	textLine := strings.Split(node.Utf8Text(tc.source), "\n")[0]
	// 	var codeLine string
	// 	if startLine < len(tc.lines) {
	// 		codeLine = tc.lines[startLine]
	// 	}
	// 	fmt.Printf("%s %s %d-%d=%d %s %s\n",
	// 		strings.Repeat("   ", depth),
	// 		node.Kind(),
	// 		startLine,
	// 		endLine,
	// 		size+1,
	// 		textLine,
	// 		codeLine,
	// 	)
	// }

	if size > 0 {
		if startLine < len(tc.header) {
			// store [size, startLine, endLine]
			tc.header[startLine] = []int{size, startLine, endLine}
		}
	}

	// Mark each line in [startLine, endLine] as belonging to scope `startLine`
	for i := startLine; i <= endLine && i < len(tc.scopes); i++ {
		tc.scopes[i][startLine] = struct{}{}
	}

	for i := uint(0); i < node.ChildCount(); i++ {
		if child := node.NamedChild(i); child != nil {
			tc.walkTree(child, depth+1)
		}
	}

	return startLine, endLine
}

// --- Helper functions ---

// mapKeysSorted returns sorted keys of a map[int]struct{} as a slice.
func mapKeysSorted(m map[int]struct{}) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// a trivial sort
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// sortNodesBySize sorts nodes by (EndLine-StartLine) descending.
func sortNodesBySize(nodes []*sitter.Node) {
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			sizeI := int(nodes[i].EndPosition().Row) - int(nodes[i].StartPosition().Row)
			sizeJ := int(nodes[j].EndPosition().Row) - int(nodes[j].StartPosition().Row)
			if sizeJ > sizeI {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
	}
}
