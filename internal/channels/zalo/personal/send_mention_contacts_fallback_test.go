package personal

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/cache"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/common"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// stubContactStore serves canned channel_contacts rows for the mention
// contacts-fallback path; counts lookups so cache-precedence can be asserted.
type stubContactStore struct {
	contacts map[string]store.ChannelContact
	lookups  atomic.Int32
}

func (s *stubContactStore) GetContactsBySenderIDs(_ context.Context, senderIDs []string) (map[string]store.ChannelContact, error) {
	s.lookups.Add(1)
	out := map[string]store.ChannelContact{}
	for _, id := range senderIDs {
		if c, ok := s.contacts[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

func (s *stubContactStore) UpsertContact(context.Context, string, string, string, string, string, string, string, string, string, string) error {
	return nil
}
func (s *stubContactStore) ListContacts(context.Context, store.ContactListOpts) ([]store.ChannelContact, error) {
	return nil, nil
}
func (s *stubContactStore) CountContacts(context.Context, store.ContactListOpts) (int, error) {
	return 0, nil
}
func (s *stubContactStore) GetContactByID(context.Context, uuid.UUID) (*store.ChannelContact, error) {
	return nil, nil
}
func (s *stubContactStore) GetSenderIDsByContactIDs(context.Context, []uuid.UUID) ([]string, error) {
	return nil, nil
}
func (s *stubContactStore) MergeContacts(context.Context, []uuid.UUID, uuid.UUID) error { return nil }
func (s *stubContactStore) UnmergeContacts(context.Context, []uuid.UUID) error          { return nil }
func (s *stubContactStore) GetContactsByMergedID(context.Context, uuid.UUID) ([]store.ChannelContact, error) {
	return nil, nil
}
func (s *stubContactStore) ResolveTenantUserID(context.Context, string, string) (string, error) {
	return "", nil
}

func contactRow(channelType, uid, displayName string) store.ChannelContact {
	return store.ChannelContact{
		ChannelType: channelType,
		SenderID:    uid,
		DisplayName: &displayName,
		ContactType: "user",
	}
}

func newChannelWithContacts(t *testing.T, rows map[string]store.ChannelContact) (*Channel, *stubContactStore) {
	t.Helper()
	ch, _ := newHandlerTestChannel(t)
	cs := &stubContactStore{contacts: rows}
	ch.SetContactCollector(store.NewContactCollector(cs, cache.NewInMemoryCache[bool]()))
	return ch, cs
}

func TestParseOutboundMentions_ContactsFallback_ResolvesColdCacheUID(t *testing.T) {
	t.Parallel()
	const uid = "583199907997701467"
	ch, cs := newChannelWithContacts(t, map[string]store.ChannelContact{
		uid: contactRow(channels.TypeZaloPersonal, uid, "Kỳ Nam"),
	})

	rendered, ms := ch.parseOutboundMentions(context.Background(), "group-1", protocol.ThreadTypeGroup, "@["+uid+"] kiểm tra giúp")
	if rendered != "@Kỳ Nam kiểm tra giúp" {
		t.Fatalf("rendered=%q", rendered)
	}
	if len(ms) != 1 || ms[0].UserID != uid || ms[0].DisplayName != "Kỳ Nam" {
		t.Fatalf("ms=%+v", ms)
	}
	if got := cs.lookups.Load(); got != 1 {
		t.Errorf("contact lookups = %d, want 1", got)
	}
}

func TestParseOutboundMentions_MemberCacheWinsOverContacts(t *testing.T) {
	t.Parallel()
	const uid = "111222333"
	ch, cs := newChannelWithContacts(t, map[string]store.ChannelContact{
		uid: contactRow(channels.TypeZaloPersonal, uid, "DB Name"),
	})
	ch.memberCache.Set("group-1", uid, "Cache Name")

	rendered, ms := ch.parseOutboundMentions(context.Background(), "group-1", protocol.ThreadTypeGroup, "@["+uid+"] hi")
	if rendered != "@Cache Name hi" {
		t.Fatalf("rendered=%q", rendered)
	}
	if len(ms) != 1 || ms[0].DisplayName != "Cache Name" {
		t.Fatalf("ms=%+v", ms)
	}
	if got := cs.lookups.Load(); got != 0 {
		t.Errorf("contacts must not be queried on member-cache hit; lookups = %d", got)
	}
}

func TestParseOutboundMentions_ContactsMiss_UIDStrippedNotLiteral(t *testing.T) {
	t.Parallel()
	ch, _ := newChannelWithContacts(t, nil)

	rendered, ms := ch.parseOutboundMentions(context.Background(), "group-1", protocol.ThreadTypeGroup, "@[999888777] Do Loi")
	if rendered != "Do Loi" {
		t.Fatalf("rendered=%q, want stripped marker", rendered)
	}
	if strings.Contains(rendered, "@[") {
		t.Fatalf("raw marker leaked: %q", rendered)
	}
	if ms != nil {
		t.Fatalf("ms=%+v, want nil", ms)
	}
}

func TestParseOutboundMentions_ContactWrongChannelType_NotUsed(t *testing.T) {
	t.Parallel()
	const uid = "444555666"
	ch, _ := newChannelWithContacts(t, map[string]store.ChannelContact{
		uid: contactRow("telegram", uid, "Telegram Person"),
	})

	rendered, _ := ch.parseOutboundMentions(context.Background(), "group-1", protocol.ThreadTypeGroup, "@["+uid+"] hi")
	if rendered != "hi" {
		t.Fatalf("rendered=%q, want cross-channel contact rejected + marker stripped", rendered)
	}
}

func TestParseOutboundMentions_NilCollector_StripsWithoutPanic(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)

	rendered, ms := ch.parseOutboundMentions(context.Background(), "group-1", protocol.ThreadTypeGroup, "@[123456] hello")
	if rendered != "hello" || ms != nil {
		t.Fatalf("rendered=%q ms=%+v", rendered, ms)
	}
}

func TestParseOutboundMentionsWithStyles_ContactsFallbackApplies(t *testing.T) {
	t.Parallel()
	const uid = "583199907997701467"
	ch, _ := newChannelWithContacts(t, map[string]store.ChannelContact{
		uid: contactRow(channels.TypeZaloPersonal, uid, "Kỳ Nam"),
	})

	// Style over "ok" at input pos 21 ("@["+uid+"] " = 21 UTF-16 units... use computed offset).
	in := "@[" + uid + "] ok"
	styles := []common.Style{{Start: len("@[") + len(uid) + len("] "), Len: 2, St: "b"}}
	rendered, ms, outStyles := ch.parseOutboundMentionsWithStyles(context.Background(), "group-1", protocol.ThreadTypeGroup, in, styles)
	if rendered != "@Kỳ Nam ok" {
		t.Fatalf("rendered=%q", rendered)
	}
	if len(ms) != 1 || ms[0].UserID != uid {
		t.Fatalf("ms=%+v", ms)
	}
	// "@Kỳ Nam " = 8 UTF-16 units → style shifts to 8.
	if len(outStyles) != 1 || outStyles[0].Start != 8 || outStyles[0].Len != 2 {
		t.Fatalf("styles=%+v want [{8,2,b}]", outStyles)
	}
}
