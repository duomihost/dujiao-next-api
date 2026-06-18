package provider

import (
	"context"
	"testing"

	"github.com/dujiao-next/internal/models"
)

type fakeProvider struct{ typ string }

func (f *fakeProvider) Type() string { return f.typ }
func (f *fakeProvider) ValidateConfig(_ models.JSON, _ string) error {
	return nil
}
func (f *fakeProvider) CreatePayment(_ context.Context, _ models.JSON, _ CreateInput) (*CreateResult, error) {
	return &CreateResult{ProviderRef: f.typ + "-ref"}, nil
}

func TestRegistry_ExactMatch(t *testing.T) {
	r := NewRegistry()
	stripeP := &fakeProvider{typ: "official:stripe"}
	r.Register("official", "stripe", stripeP)

	got, ok := r.Lookup("official", "stripe")
	if !ok {
		t.Fatalf("expected stripe lookup ok")
	}
	if got != stripeP {
		t.Fatalf("got wrong provider, want stripe instance")
	}
}

func TestRegistry_FallbackToProviderTypeOnly(t *testing.T) {
	r := NewRegistry()
	epayP := &fakeProvider{typ: "epay"}
	r.Register("epay", "", epayP)

	got, ok := r.Lookup("epay", "wxpay") // 精确 epay:wxpay 没有,fallback epay:
	if !ok {
		t.Fatalf("expected fallback lookup ok")
	}
	if got != epayP {
		t.Fatalf("got wrong provider, want epay instance")
	}
}

func TestRegistry_CaseInsensitive(t *testing.T) {
	r := NewRegistry()
	p := &fakeProvider{typ: "official:stripe"}
	r.Register("OFFICIAL", "Stripe", p)

	got, ok := r.Lookup("official", "stripe")
	if !ok || got != p {
		t.Fatalf("lookup should be case-insensitive")
	}
}

func TestRegistry_Miss(t *testing.T) {
	r := NewRegistry()
	r.Register("official", "stripe", &fakeProvider{typ: "stripe"})

	if _, ok := r.Lookup("official", "unknown"); ok {
		t.Fatalf("expected miss for official:unknown(no fallback because official: not registered)")
	}
	if _, ok := r.Lookup("nonexistent", "x"); ok {
		t.Fatalf("expected miss for nonexistent:x")
	}
}
