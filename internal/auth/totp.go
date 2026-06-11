package auth

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"strings"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// ErrInvalidTOTP is returned for a wrong or expired two-factor code.
var ErrInvalidTOTP = errors.New("invalid two-factor code")

// totpIssuer labels the account in authenticator apps.
const totpIssuer = "CronPilot"

// VerifyTOTP reports whether code is currently valid for secret. An empty
// secret or code never validates. totp.Validate accepts the adjacent time
// steps, tolerating modest clock skew.
func VerifyTOTP(secret, code string) bool {
	secret = strings.TrimSpace(secret)
	code = strings.TrimSpace(code)
	if secret == "" || code == "" {
		return false
	}
	return totp.Validate(code, secret)
}

// SetupTOTP generates and stores a new (not yet enforced) TOTP secret for the
// user and returns the secret plus the otpauth:// provisioning URL. Enforcement
// only begins once EnableTOTP confirms a code, so calling this again before
// confirming simply rotates the pending secret.
func (m *Manager) SetupTOTP(userID string) (secret, url string, err error) {
	user, err := m.store.GetUserByID(userID)
	if err != nil {
		return "", "", err
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: user.Username,
	})
	if err != nil {
		return "", "", err
	}
	// Stored disabled: 2FA is only enforced after EnableTOTP confirms a code.
	if err := m.store.UpdateUserTOTP(userID, key.Secret(), false); err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// EnableTOTP confirms a code against the user's pending secret and, on success,
// turns on enforcement. Returns ErrInvalidTOTP if no secret has been set up or
// the code is wrong.
func (m *Manager) EnableTOTP(userID, code string) error {
	user, err := m.store.GetUserByID(userID)
	if err != nil {
		return err
	}
	if user.TOTPSecret == "" || !VerifyTOTP(user.TOTPSecret, code) {
		return ErrInvalidTOTP
	}
	return m.store.UpdateUserTOTP(userID, user.TOTPSecret, true)
}

// DisableTOTP clears a user's TOTP secret and turns off enforcement. The current
// password is required so a hijacked session cannot silently strip 2FA.
func (m *Manager) DisableTOTP(userID, password string) error {
	user, err := m.store.GetUserByID(userID)
	if err != nil {
		return err
	}
	ok, err := VerifyPassword(user.PasswordHash, password)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}
	return m.store.UpdateUserTOTP(userID, "", false)
}

// QRCodeDataURI renders an otpauth:// URL as a PNG QR code, returned as a data:
// URI suitable for an <img> src. Authenticator apps scan it to import the secret
// without manual entry.
func QRCodeDataURI(otpauthURL string) (string, error) {
	key, err := otp.NewKeyFromURL(otpauthURL)
	if err != nil {
		return "", err
	}
	img, err := key.Image(240, 240)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
