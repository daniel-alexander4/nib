# docs/red-proofs.md, tier 3: "The compare hash's margin was inverted by an illumination
# gradient's SIGN" (/pending 276, v1.117.139)
#
# The defect: `dhashFromGrid` emits one strict `lum(here) > lum(right)` per adjacent pair, 64
# bits. A page is mostly paper, so ~22 of the 64 pairs are TIES, and a strict `>` files every
# one of them under "darker". An illumination gradient that brightens left-to-right therefore
# moves nothing at all, and one that darkens flips all 22 at once — the same page under the same
# lamp, tilted the other way, measured 0 bits and 25 bits from its clean render against a
# threshold of 12. A genuinely different page sat at 10. No threshold separates those, which is
# why this was fixed rather than retuned.
#
# The fix compares log-ratios (a multiplicative illumination change shifts every pair alike
# whatever its brightness) and emits a TRIT against a band taken from the page's own distribution,
# so "neither" is its own symbol and ties stop being counted as "darker". 128 bits.
#
# **Tier 3 and not tier 2**: `test/jsdom/boot.mjs` names "no canvas … the compare pixel map" as
# its ceiling. There is nothing for the reduction to draw into, and the whole measurement is of
# what a real Chromium render reduces to.
TIER="tier 3 — the real binary in a real browser"
PROVE="./build/uirepro.sh"
EXPECT="a scan of one page under an uneven lamp does not match itself"
