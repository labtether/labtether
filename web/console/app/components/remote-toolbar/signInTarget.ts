type SignInAsset = { id: string; name?: string; platform?: string; metadata?: Record<string, string> };

/**
 * The device's name when it is a Mac, for the sign-in card: Apple Screen Sharing asks for a username and password of
 * an account on that Mac, not a VNC password. Undefined for other devices and for a host that is not a device.
 */
export function macSignInName(assets: SignInAsset[] | undefined, assetId: string | undefined): string | undefined {
  const asset = assetId ? assets?.find((entry) => entry.id === assetId) : undefined;
  if (!asset) return undefined;
  const platform = `${asset.platform ?? ""} ${asset.metadata?.platform ?? ""}`;
  return /darwin|macos/i.test(platform) ? asset.name || asset.id : undefined;
}
