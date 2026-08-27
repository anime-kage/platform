package main

import (
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"animekage/backend/internal/config"
)

func testBot() *bot {
	return newBot(&config.BotConfig{
		ChannelID:           "100",
		ProtectionChannelID: "200",
		CommandName:         "invitatie",
		PublicURL:           "https://anime-kage.ro",
		InviteTTL:           7 * 24 * time.Hour,
		QuotaWindow:         24 * time.Hour,
		PurgeWindow:         5 * time.Minute,
		MutedRoleID:         "300",
	}, nil)
}

// Nobody can edit a bot's messages from the Discord client, not even the server
// owner — these strings are the only copy of that text, so they are worth a
// test. The em dash is the specific thing being kept out: it is not how the rest
// of the site writes Romanian, and it reads as machine-written.
func TestMemberFacingTextAvoidsDashes(t *testing.T) {
	b := testBot()
	for name, text := range map[string]string{
		"boardText":        b.boardText(),
		"protectionNotice": b.protectionNotice(),
	} {
		for _, bad := range []string{"—", "–", " -- "} {
			if strings.Contains(text, bad) {
				t.Errorf("%s contains %q:\n%s", name, bad, text)
			}
		}
	}
}

func TestBoardTextCarriesTheMarkerAndTheRules(t *testing.T) {
	// The marker is how ensureMessage recognises a board it posted before. Lose
	// it from the text and every restart posts a fresh board next to the old one.
	got := testBot().boardText()
	for _, want := range []string{boardMarker, "https://anime-kage.ro/register", "7 zile", "o dată pe zi", "/invitatie"} {
		if !strings.Contains(got, want) {
			t.Errorf("boardText missing %q:\n%s", want, got)
		}
	}
}

func TestProtectionNoticeSaysWhatHappens(t *testing.T) {
	got := testBot().protectionNotice()
	for _, want := range []string{protectionMarker, "MUT PERMANENT", "5 minute"} {
		if !strings.Contains(got, want) {
			t.Errorf("protectionNotice missing %q:\n%s", want, got)
		}
	}

	// Without a mute role the trap can only hand out a 28-day timeout, and the
	// sign must not go on promising something permanent.
	b := testBot()
	b.cfg.MutedRoleID = ""
	if strings.Contains(b.protectionNotice(), "PERMANENT") {
		t.Errorf("notice promises a permanent mute with no mute role configured:\n%s", b.protectionNotice())
	}

	// It is a sign, not a policy document. Anything longer stops being read,
	// which is the only thing it has to achieve.
	if lines := strings.Count(got, "\n") + 1; lines > 6 {
		t.Errorf("protectionNotice grew to %d lines:\n%s", lines, got)
	}
}

// A burst in the honeypot is the normal case, not the exception: a spam script
// posts several times before the mute takes effect, and each message arrives as
// its own event. Only the first may start a purge.
func TestClaimPunishCollapsesABurst(t *testing.T) {
	b := testBot()
	if !b.claimPunish("user-1") {
		t.Fatal("first message did not claim the punishment")
	}
	for i := 0; i < 5; i++ {
		if b.claimPunish("user-1") {
			t.Fatalf("message %d started a second purge", i+2)
		}
	}
	if !b.claimPunish("user-2") {
		t.Error("a different account was swallowed by the cooldown")
	}
}

func TestClaimPunishForgetsAfterTheCooldown(t *testing.T) {
	b := testBot()
	b.claimPunish("user-1")

	b.mu.Lock()
	b.punished["user-1"] = time.Now().Add(-punishCooldown - time.Minute)
	b.mu.Unlock()

	if !b.claimPunish("user-1") {
		t.Fatal("an account that tripped the trap again much later was ignored")
	}
	// The sweep runs under the same lock and must not have dropped the entry it
	// just wrote, or the cooldown would never hold.
	b.mu.Lock()
	_, ok := b.punished["user-1"]
	b.mu.Unlock()
	if !ok {
		t.Error("claimPunish swept away the entry it just recorded")
	}
}

func TestHasMessagesSkipsWhatCannotHoldSpam(t *testing.T) {
	// Categories and forum roots hold no messages of their own; forum posts live
	// in threads, which GuildThreadsActive returns separately. Voice channels do
	// have a text chat, and a spam script walking the channel list will use it.
	cases := map[discordgo.ChannelType]bool{
		discordgo.ChannelTypeGuildText:          true,
		discordgo.ChannelTypeGuildNews:          true,
		discordgo.ChannelTypeGuildVoice:         true,
		discordgo.ChannelTypeGuildPublicThread:  true,
		discordgo.ChannelTypeGuildPrivateThread: true,
		discordgo.ChannelTypeGuildCategory:      false,
		discordgo.ChannelTypeGuildForum:         false,
		discordgo.ChannelTypeGuildStageVoice:    false,
	}
	for typ, want := range cases {
		if got := hasMessages(&discordgo.Channel{Type: typ}); got != want {
			t.Errorf("hasMessages(type %d) = %v, want %v", typ, got, want)
		}
	}
}

// signalRestick is written to from every message event in the invite channel and
// must never block the gateway goroutine, however many arrive.
func TestSignalRestickNeverBlocks(t *testing.T) {
	b := testBot()
	for i := 0; i < 1000; i++ {
		b.signalRestick()
	}
	if len(b.restick) != 1 {
		t.Errorf("pending resticks = %d, want 1", len(b.restick))
	}
}
