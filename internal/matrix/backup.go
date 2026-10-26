package matrix

import (
	"context"
	"errors"
	"fmt"

	"maunium.net/go/mautrix/crypto/backup"
	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/event"
)

// Recover verifies this device with the account's recovery key (it signs the device with the
// cross-signing keys kept in secret storage) and imports every room key from the server key backup,
// so messages sent before this login decrypt. Parked messages retry on the next sync.
func (c *Client) Recover(ctx context.Context, recoveryKey string) error {
	mach := c.helper.Machine()
	if err := mach.VerifyWithRecoveryKey(ctx, recoveryKey); err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	keyID, keyData, err := mach.SSSS.GetDefaultKeyData(ctx)
	if err != nil {
		return fmt.Errorf("secret storage: %w", err)
	}
	key, err := keyData.VerifyRecoveryKey(keyID, recoveryKey)
	if err != nil && !errors.Is(err, ssss.ErrUnverifiableKey) { // unverifiable is tolerated, as mautrix does
		return fmt.Errorf("recovery key: %w", err)
	}
	raw, err := mach.SSSS.GetDecryptedAccountData(ctx, event.AccountDataMegolmBackupKey, key)
	if err != nil {
		return fmt.Errorf("backup key: %w", err)
	}
	bk, err := backup.MegolmBackupKeyFromBytes(raw)
	if err != nil {
		return fmt.Errorf("backup key: %w", err)
	}
	if _, err := mach.DownloadAndStoreLatestKeyBackup(ctx, bk); err != nil {
		return fmt.Errorf("key backup: %w", err)
	}
	return nil
}
