package gateway

import (
	"errors"
	"net"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/oboo/moho/internal/tun"
)

// fake is a system whose forwarding is a variable: nothing here touches
// the Mac's.
type fake struct {
	fwd      bool
	ready    bool
	sets     []bool // what SetForwarding was asked, in order
	awake    int    // KeepAwake calls not stopped yet
	started  int
	prepared [][]tun.Step
}

func (f *fake) Forwarding() (bool, error) { return f.fwd, nil }
func (f *fake) SetForwarding(on bool) error {
	if !f.ready {
		return errors.New("not authorised")
	}
	f.sets = append(f.sets, on)
	f.fwd = on
	return nil
}
func (f *fake) Ready() bool { return f.ready }
func (f *fake) Prepare(prompt string, before ...tun.Step) error {
	f.prepared = append(f.prepared, before)
	f.ready = true
	return nil
}
func (f *fake) KeepAwake() (func(), error) {
	f.awake++
	f.started++
	return func() { f.awake-- }, nil
}
func (f *fake) Network() Network            { return Network{IP: "192.168.1.20", Iface: "en0"} }
func (f *fake) AnswersDNS(addr string) bool { return true }

func TestEnableRestores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway-before.json")
	f := &fake{ready: true}
	g := New(f, path)
	if err := g.Enable(); err != nil {
		t.Fatal(err)
	}
	if !f.fwd || f.awake != 1 || !g.Pending() || !g.Awake() {
		t.Fatalf("after Enable: forwarding %v, awake %d, pending %v", f.fwd, f.awake, g.Pending())
	}
	// Again: nothing new, and the original stays what it was (off), not
	// the on Enable made.
	if err := g.Enable(); err != nil {
		t.Fatal(err)
	}
	if len(f.sets) != 1 || f.started != 1 {
		t.Fatalf("second Enable: sets %v, keep-awake started %d times", f.sets, f.started)
	}
	if err := g.Disable(); err != nil {
		t.Fatal(err)
	}
	if f.fwd || f.awake != 0 || g.Pending() || g.Awake() {
		t.Fatalf("after Disable: forwarding %v, awake %d, pending %v", f.fwd, f.awake, g.Pending())
	}
	// Nothing to put back now.
	if err := g.Disable(); err != nil || len(f.sets) != 2 {
		t.Fatalf("second Disable: %v, sets %v", err, f.sets)
	}
}

// Forwarding that was on before the gateway (Internet Sharing, say) is
// not the gateway's to turn off.
func TestEnableKeepsForwardingThatWasOn(t *testing.T) {
	f := &fake{ready: true, fwd: true}
	g := New(f, filepath.Join(t.TempDir(), "b.json"))
	if err := g.Enable(); err != nil {
		t.Fatal(err)
	}
	if err := g.Disable(); err != nil {
		t.Fatal(err)
	}
	if !f.fwd || len(f.sets) != 0 {
		t.Fatalf("forwarding %v, sets %v", f.fwd, f.sets)
	}
}

// After a crash the file is still there: a new start puts forwarding back.
func TestDisableAfterCrash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.json")
	f := &fake{ready: true}
	if err := New(f, path).Enable(); err != nil {
		t.Fatal(err)
	}
	f.awake = 0 // the process that kept it awake died with the app
	g := New(f, path)
	if !g.Pending() {
		t.Fatal("a new gateway doesn't see the forwarding left on")
	}
	if err := g.Disable(); err != nil {
		t.Fatal(err)
	}
	if f.fwd || g.Pending() {
		t.Fatalf("forwarding %v, pending %v", f.fwd, g.Pending())
	}
}

// Disable on a gateway that never ran touches nothing.
func TestDisableUntouched(t *testing.T) {
	f := &fake{ready: true, fwd: true}
	if err := New(f, filepath.Join(t.TempDir(), "b.json")).Disable(); err != nil {
		t.Fatal(err)
	}
	if !f.fwd || len(f.sets) != 0 {
		t.Fatalf("forwarding %v, sets %v", f.fwd, f.sets)
	}
}

func TestEnableNotReady(t *testing.T) {
	f := &fake{}
	g := New(f, filepath.Join(t.TempDir(), "b.json"))
	if err := g.Enable(); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Enable without the authorisation: %v", err)
	}
	if f.fwd || f.awake != 0 {
		t.Fatalf("forwarding %v, awake %d", f.fwd, f.awake)
	}
	if ErrNotReady.Error() == "" {
		t.Error("ErrNotReady says nothing")
	}
}

func TestDevices(t *testing.T) {
	locals := []netip.Addr{netip.MustParseAddr("192.168.1.20"), netip.MustParseAddr("198.18.0.1")}
	src := []string{
		"192.168.1.31", "192.168.1.31", // one phone, two connections
		"192.168.1.40",
		"192.168.1.20", "198.18.0.1", "127.0.0.1", "::1", // this Mac
		"", "bogus",
		"::ffff:192.168.1.40", // the same TV, as IPv4-mapped
	}
	if n := Devices(src, locals); n != 2 {
		t.Fatalf("Devices = %d, want 2", n)
	}
	if n := Devices(nil, locals); n != 0 {
		t.Fatalf("Devices(nil) = %d", n)
	}
}

// answersDNS against a resolver of the test's own, on a free port.
func TestAnswersDNS(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 512)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			resp := append([]byte{}, buf[:n]...)
			resp[2] |= 0x80 // QR: a response
			_, _ = pc.WriteTo(resp, from)
		}
	}()
	if !answersDNS(pc.LocalAddr().String(), time.Second) {
		t.Error("the resolver's answer was not seen")
	}
	// A port nothing answers on.
	quiet, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := quiet.LocalAddr().String()
	if answersDNS(addr, 200*time.Millisecond) {
		t.Error("silence counts as an answer")
	}
	quiet.Close()

	q := dnsQuery(0x1234, "apple.com")
	want := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 5, 'a', 'p', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0, 0, 1, 0, 1}
	if string(q) != string(want) {
		t.Errorf("dnsQuery = %v", q)
	}
	if isAnswer(q, 0x1234) || !isAnswer(append([]byte{0x12, 0x34, 0x81}, q[3:]...), 0x1234) || isAnswer([]byte{1}, 1) {
		t.Error("isAnswer")
	}
}

func TestPhysical(t *testing.T) {
	for name, want := range map[string]bool{"en0": true, "en7": true, "utun13": false, "bridge0": false, "lo0": false, "awdl0": false, "": false} {
		if physical(name) != want {
			t.Errorf("physical(%q) = %v", name, !want)
		}
	}
}
