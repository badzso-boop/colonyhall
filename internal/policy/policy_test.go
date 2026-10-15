package policy

import "testing"

func TestChannelRules(t *testing.T) {
	admin := Subject{IsAdmin: true}
	human := Subject{}
	agent := Subject{IsAgent: true}
	none := Membership{}
	reader := Membership{IsMember: true}
	writer := Membership{IsMember: true, CanWrite: true}

	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"admin reads without membership", CanReadChannel(admin, none), true},
		{"non-member cannot read", CanReadChannel(human, none), false},
		{"member reads", CanReadChannel(agent, reader), true},
		{"admin writes without membership", CanWriteChannel(admin, none), true},
		{"read-only member cannot write", CanWriteChannel(agent, reader), false},
		{"writer writes", CanWriteChannel(agent, writer), true},
		{"human creates channel", CanCreateChannel(human), true},
		{"agent cannot create channel", CanCreateChannel(agent), false},
		{"only admin manages members", CanManageMembers(human), false},
		{"halted agent cannot write", CanWriteChannel(Subject{IsAgent: true, Halted: true}, writer), false},
		{"halted admin cannot write", CanWriteChannel(Subject{IsAdmin: true, Halted: true}, writer), false},
		{"halted human cannot create channel", CanCreateChannel(Subject{Halted: true}), false},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}
