package show

// TODO(macro): Drop the Manager alias once callers use ShowManager — a dual name
// for the same type blurs the package's primary aggregate and keeps a
// compatibility layer without a migration owner.
// Manager is the application's show state manager. ShowManager remains the
// underlying name for compatibility with existing package APIs.
// TODO(micro): type alias with no migration deadline; grep callers and delete Manager once ShowManager is universal
type Manager = ShowManager
