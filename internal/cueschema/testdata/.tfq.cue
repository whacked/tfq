status:        "pending" | "in-progress" | "completed" | "blocked" | "cancelled"
priority?:     "low" | "medium" | "high" | "critical"
dependencies?: [...string] @edge(blocking)
parent?:       string      @edge()
