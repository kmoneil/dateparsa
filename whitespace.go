package dateparsa

// Surrounding whitespace is padding, and padding is a property of the row
// rather than of the format.
//
// A CSV column is padded, a log line ends in CRLF, and a spreadsheet export
// pads inconsistently: row 1 arrives as "2024-03-15" and row 900 as
// "2024-03-15 ". README.md names both use cases in its first paragraph, and
// until F9 the library refused every one of those rows on the numeric half of
// the format table while accepting them on the textual half. The tolerance was
// not a decision: textual formats locate their fields by scanning and coverGaps
// turns whatever is left over into a skip, and natural language skips space,
// tab and comma while tokenising. The trie matches a signature over the whole
// input and whitespace is not in the signature.
//
// So this is that decision, made once and in one place, and what it excludes is
// the more interesting half.

// isPadding reports whether c is one of the six ASCII bytes this library treats
// as padding: space, tab, newline, carriage return, vertical tab and form feed.
//
// It is unicode.IsSpace minus everything that is not ASCII, and the omission is
// deliberate rather than an oversight about encoding.
//
// U+00A0 NO-BREAK SPACE is the one worth arguing about. It arrives from Word,
// from Excel and from copy-paste, which are the same sources as the padded
// column this exists for. It is excluded because it is a character rather than
// layout: it is written to stop a line breaking, a value that contains one is
// arguably not the value, and admitting it turns a byte loop into a decode.
// Every wider reading is worse: U+2007, U+202F and the rest of Zs would make
// trimming a Unicode table question in a library that imports neither regexp
// nor unicode today, and the binary-size budget is 10MB with twenty locales
// already in it.
//
// strings.TrimSpace is not used for the same reason: it trims Unicode space,
// NBSP included, which is the decision above taken the other way by accident.
//
// Written as two comparisons rather than six, because the six are ' ' and the
// contiguous run \t \n \v \f \r, which is 0x09 through 0x0D. That is not
// premature: Layout.Parse tests the first and last byte of every input it is
// handed, and Layout_Parse_ISODate is 18ns, so this function's body is a
// measurable fraction of the operation the library exists to be fast at.
func isPadding(c byte) bool {
	return c == ' ' || (c >= '\t' && c <= '\r')
}

// trimPadding returns s without leading or trailing padding.
//
// It is a subslice, so it allocates nothing and TestLayoutParseZeroAlloc is not
// at risk. Both loops are bounded against each other rather than against
// len(s), so an input that is entirely padding returns "" instead of running
// off either end, and "" is refused by every detector downstream.
//
// Cost is linear in the padding and not in the input: an unpadded value fails
// both loop conditions on the first byte tested and returns s itself. It is
// small enough to inline, which is what makes it usable at the detection entry
// where it is called unconditionally.
//
// Layout.Parse does not call it unconditionally, and the comment there says
// why: even inlined, entering the loops costs enough to show on an 18ns
// operation. That is measured rather than assumed, and the numbers are in the
// commit that added this.
func trimPadding(s string) string {
	i, j := 0, len(s)
	for i < j && isPadding(s[i]) {
		i++
	}
	for j > i && isPadding(s[j-1]) {
		j--
	}
	return s[i:j]
}

// trimPaddingBytes is trimPadding for the ParseBytes path.
//
// It trims before the copy to a string rather than after, so a padded row
// copies fewer bytes than it did and can cross back under stringCopyStackMax,
// which is where ExecuteBytes stops allocating. A padded input never costs an
// allocation this way that its trimmed form does not, and
// TestLayoutParseZeroAlloc has a sample on each side of that boundary.
func trimPaddingBytes(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && isPadding(b[i]) {
		i++
	}
	for j > i && isPadding(b[j-1]) {
		j--
	}
	return b[i:j]
}

// hasPadding reports whether s begins or ends with padding, which is the test
// Layout.Parse runs on every row instead of trimming every row.
//
// It is separate from trimPadding so that the hot path pays two byte loads and
// four comparisons rather than the setup for two loops, and so that both entry
// points spell the guard the same way. Folding it into trimPadding does not
// work: the combined function grows past the inlining budget and the caller
// pays a call, which measured worse than either.
func hasPadding(s string) bool {
	return len(s) != 0 && (isPadding(s[0]) || isPadding(s[len(s)-1]))
}

// hasPaddingBytes is hasPadding for the ParseBytes path.
func hasPaddingBytes(b []byte) bool {
	return len(b) != 0 && (isPadding(b[0]) || isPadding(b[len(b)-1]))
}
