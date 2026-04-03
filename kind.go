package dateparsa

// Kind describes what class of date expression was parsed.
type Kind int

const (
	KindAbsolute Kind = iota // Structured date: "2024-03-15", "March 15, 2024"
	KindRelative             // Relative expression: "3 days ago", "yesterday"
	KindNow                  // Current time reference: "now", "today"
)

func (k Kind) String() string {
	switch k {
	case KindAbsolute:
		return "absolute"
	case KindRelative:
		return "relative"
	case KindNow:
		return "now"
	default:
		return "unknown"
	}
}
