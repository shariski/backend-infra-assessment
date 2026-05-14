package hash

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	pw := "s3cret-password"
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if h == pw {
		t.Fatal("HashPassword() returned the plaintext password")
	}
	if err := CheckPassword(h, pw); err != nil {
		t.Errorf("CheckPassword() with correct password error = %v", err)
	}
	if err := CheckPassword(h, "wrong-password"); err == nil {
		t.Error("CheckPassword() with wrong password expected error, got nil")
	}
}

func TestSHA256Hex(t *testing.T) {
	got := SHA256Hex("hello")
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("SHA256Hex(%q) = %q, want %q", "hello", got, want)
	}
	if SHA256Hex("hello") != SHA256Hex("hello") {
		t.Error("SHA256Hex is not deterministic")
	}
}
