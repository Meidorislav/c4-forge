// English UI strings. This catalog is the source of truth for translation
// keys: other languages must provide the same keys.
export const en = {
  app: {
    name: "c4-forge",
    tagline: "Model your software architecture with C4, together.",
  },
  auth: {
    loading: "Checking your session…",
    httpsRequired: "A secure HTTPS connection is required to sign in.",
    unavailable: "Authentication is unavailable. Please try again.",
    setupTitle: "Set up your administrator account",
    setupDescription: "Enter your email and the setup secret configured by your administrator.",
    passwordHint: "Use at least 15 characters. Spaces and passphrases are welcome.",
    email: "Email",
    password: "Password",
    setupSecret: "Setup secret",
    createAdmin: "Create administrator",
    signIn: "Sign in",
    signOut: "Sign out",
    signedIn: "Your account",
    signedInAs: "Signed in as {{email}}",
    failed: "The email, password or setup secret is incorrect.",
    invalidInput:
      "Check your email and choose a password of at least 15 characters that is not commonly used.",
    setupUnavailable:
      "Setup is unavailable. Check the server configuration or sign in if setup is complete.",
    rateLimited: "Too many attempts. Please wait before trying again.",
  },
  users: {
    title: "User management",
    role: "Role",
    admin: "Administrator",
    user: "User",
    create: "Create user",
    disable: "Disable",
    disabled: "(disabled)",
    reset: "Reset password",
    credentialHint: "Share this temporary password securely. It will only be shown once.",
    expires: "Expires at {{value}}",
    changeTitle: "Choose a new password",
    newPassword: "New password",
    changePassword: "Change password",
    error: "The operation failed. Please try again.",
    conflict: "This operation is unavailable; check the account and active administrators.",
  },
  status: {
    connecting: "Connecting to the server…",
    version: "Server version {{version}}",
    unavailable: "The server is unavailable.",
  },
} as const;
