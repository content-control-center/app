package zernio

// Settings keys used by the integration. All keys live under the
// "zernio." prefix so the metadata cache (Phase 4) can detect them with
// a single prefix check on writes.
const (
	SettingProfileID        = "zernio.profile_id"
	SettingProfileName      = "zernio.profile_name"
	SettingProfileCreatedAt = "zernio.profile_created_at"
	SettingProfileMeta      = "zernio.profile_meta"
	SettingLastSyncAt       = "zernio.last_sync_at"
	SettingLastSyncStatus   = "zernio.last_sync_status"
	// Analytics refresh health (CON-93 §11). These live in the Zernio
	// adapter namespace, so they stay `zernio.`-prefixed even though the
	// post-side columns are publisher-agnostic.
	SettingAnalyticsLastRefreshAt     = "zernio.analytics.last_refresh_at"
	SettingAnalyticsLastRefreshStatus = "zernio.analytics.last_refresh_status"
)

// SettingPrefix is the namespace shared by every Zernio-managed key.
// Used by the cache invalidator to recognise Zernio writes.
const SettingPrefix = "zernio."

// ManagedProfileName is the canonical name written to Zernio when Ogen
// creates the profile, and the fallback lookup key when no profile ID
// is stored in settings. An admin renaming the profile on the Zernio
// dashboard breaks the name-based fallback — see the bootstrap
// concurrent-boot note for the mitigation.
const (
	ManagedProfileName        = "Ogen integration"
	ManagedProfileDescription = "Auto-managed by Ogen — do not rename or delete"
)
