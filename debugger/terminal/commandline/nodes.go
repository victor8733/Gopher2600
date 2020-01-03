package commandline

import (
	"fmt"
	"strings"
)

type nodeType int

const (
	nodeRoot nodeType = iota + 1
	nodeRequired
	nodeOptional
)

// nodes are chained together throught the next and branch arrays.
type node struct {
	// tag should be non-empty - except in the case of some nested groups
	tag string

	// friendly name for the placeholder tags. not used if tag is not a
	// placeholder. you can use isPlaceholder() to check
	placeholderLabel string

	typ nodeType

	next   []*node
	branch []*node

	repeatStart bool
	repeat      *node
}

// String returns the verbose representation of the node (and its children).
// Use this only for testing/validation purposes. HelpString() is more useful
// to the end user.
func (n node) String() string {
	return n.outerString(false)
}

// HelpString returns the string representation of the node (and it's children)
// without extraneous placeholder directives (if placeholderLabel is available)
//
// So called because it's better to use when displaying help
func (n node) usageString() string {
	return n.outerString(true)
}

func (n node) outerString(preferLabels bool) string {
	s := strings.Builder{}

	if n.repeatStart {
		s.WriteString("{")
	} else if n.typ == nodeOptional {
		s.WriteString("(")
		defer func() {
			s.WriteString(")")
		}()
	}
	if n.typ == nodeRequired {
		s.WriteString("[")
		defer func() {
			s.WriteString("]")
		}()
	}

	s.WriteString(n.innerString(preferLabels))
	return s.String()
}

// innerString() outputs the node, and any children, as best as it can. when called
// upon the first node in a command it has the effect of recreating the
// original input to each template entry parsed by ParseCommandTemplate()
//
// however, because of the way the parsing works, it's not always possible to
// recreate accurately the original template entry, but that's okay. the node
// tree is effectively, an optimised tree and so the output from String() is
// likewise, optimised
//
// optimised in this case means the absence of superfluous group indicators.
// for example:
//
//		TEST [1 [2] [3] [4] [5]]
//
// is the same as:
//
//		TEST [1 2 3 4 5]
//
// note: innerString should not be called directly except as a recursive call
// or as an initial call from String()
//
func (n node) innerString(preferLabels bool) string {
	s := strings.Builder{}

	if n.isPlaceholder() && n.placeholderLabel != "" {
		// placeholder labels come without angle brackets
		label := fmt.Sprintf("<%s>", n.placeholderLabel)
		if preferLabels {
			s.WriteString(label)
		} else {
			s.WriteString(fmt.Sprintf("%%%s%c", label, n.tag[1]))
		}
	} else {
		s.WriteString(n.tag)
	}

	if n.next != nil {
		for i := range n.next {
			prefix := " "
			if n.next[i].repeatStart {
				s.WriteString(" {")
				prefix = ""
			}

			if n.next[i].typ == nodeRequired && (n.typ != nodeRequired || n.next[i].branch != nil) {
				s.WriteString(prefix)
				s.WriteString("[")
			} else if n.next[i].typ == nodeOptional && (n.typ != nodeOptional || n.next[i].branch != nil) {
				// repeat groups are optional groups by definition so we don't
				// need to include the optional group delimiter
				if !n.next[i].repeatStart {
					s.WriteString(prefix)
					s.WriteString("(")
				}
			} else {
				s.WriteString(prefix)
			}

			s.WriteString(n.next[i].innerString(preferLabels))

			if n.next[i].typ == nodeRequired && (n.typ != nodeRequired || n.next[i].branch != nil) {
				s.WriteString("]")
			} else if n.next[i].typ == nodeOptional && (n.typ != nodeOptional || n.next[i].branch != nil) {
				// see comment above
				if !n.next[i].repeatStart {
					s.WriteString(")")
				}
			}

		}
	}

	if n.branch != nil {
		for i := range n.branch {
			s.WriteString(fmt.Sprintf("|%s", n.branch[i].innerString(preferLabels)))
		}
	}

	// unlike the other close group delimiters, we add the close repeat group
	// here. this is the best way of making sure we add exactly one close
	// delimiter for every open delimiter.
	if n.repeatStart {
		s.WriteString("}")
	}

	return strings.TrimSpace(s.String())
}

// nodeVerbose returns a readable representation of the node, listing branches
// if necessary
func (n node) nodeVerbose() string {
	s := strings.Builder{}
	s.WriteString(n.tagVerbose())
	for bi := range n.branch {
		if n.branch[bi].tag != "" {
			s.WriteString(" or ")
			s.WriteString(n.branch[bi].tagVerbose())
		}
	}
	return s.String()
}

// tagVerbose returns a readable versions of the tag field, using labels if
// possible
func (n node) tagVerbose() string {
	if n.isPlaceholder() {
		if n.placeholderLabel != "" {
			return n.placeholderLabel
		}

		switch n.tag {
		case "%S":
			return "string argument"
		case "%N":
			return "numeric argument"
		case "%P":
			return "floating-point argument"
		case "%F":
			return "filename argument"
		default:
			return "placeholder argument"
		}
	}
	return n.tag
}

// isPlaceholder checks tag to see if it is a placeholder. does not check to
// see if placeholder is valid
func (n node) isPlaceholder() bool {
	return len(n.tag) == 2 && n.tag[0] == '%'
}
