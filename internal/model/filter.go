package model

import "sort"

// Filter holds optional criteria for listing tickets.
type Filter struct {
	Status   Status
	Priority Priority
	Provider string
	Label    string
}

// Match returns true if the ticket passes the filter.
func (f Filter) Match(t Ticket) bool {
	if f.Status != "" && t.Status != f.Status {
		return false
	}
	if f.Priority != "" && t.Priority != f.Priority {
		return false
	}
	if f.Provider != "" && t.Provider != f.Provider {
		return false
	}
	if f.Label != "" {
		found := false
		for _, l := range t.Labels {
			if l == f.Label {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// SortByPriorityThenAge sorts tickets by priority (urgent first) then creation date (oldest first).
func SortByPriorityThenAge(tickets []Ticket) {
	sort.Slice(tickets, func(i, j int) bool {
		ri := PriorityRank(tickets[i].Priority)
		rj := PriorityRank(tickets[j].Priority)
		if ri != rj {
			return ri < rj
		}
		return tickets[i].CreatedAt.Before(tickets[j].CreatedAt)
	})
}
