// Package policy decides who may do what. Every function is pure: callers
// pass in the facts (role, membership) and get a yes or no, so the rules
// are tested without a database (Stoat core/permissions pattern).
package policy

// Subject is the acting party as far as permissions are concerned.
type Subject struct {
	IsAdmin bool
	IsAgent bool
	// Halted: paused, or an agent during an emergency stop. Halted
	// subjects may read but never write.
	Halted bool
}

// Membership is the subject's membership in a channel, if any.
type Membership struct {
	IsMember bool
	CanWrite bool
}

// CanReadChannel: admins see every channel; everyone else needs membership.
func CanReadChannel(s Subject, m Membership) bool {
	return s.IsAdmin || m.IsMember
}

// CanWriteChannel: admins may always write (to intervene anywhere);
// members need write permission.
func CanWriteChannel(s Subject, m Membership) bool {
	if s.Halted {
		return false
	}
	return s.IsAdmin || (m.IsMember && m.CanWrite)
}

// CanCreateChannel: humans may create channels; agents may not.
func CanCreateChannel(s Subject) bool {
	return !s.IsAgent && !s.Halted
}

// CanManageMembers: only admins add or remove members.
func CanManageMembers(s Subject) bool {
	return s.IsAdmin
}
