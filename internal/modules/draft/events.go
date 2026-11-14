package draft

// EventChanged tells the owner's other tabs and devices that a draft was
// saved or removed. It carries the place only, never the text, and the
// gateway sends it to the owner alone.
const EventChanged = "draft.changed"

// Changed is the payload of EventChanged.
type Changed struct {
	ChannelID    string `json:"channel_id"`
	ThreadRootID string `json:"thread_root_id,omitempty"`
	Deleted      bool   `json:"deleted"`
}
