# docs/red-proofs.md, tier 3: "the panel pills keep the tab strip's square corners" (v1.123.2)
#
# `.sbhead`'s radius is deleted, so the PANEL cards fall back to `.tab { border-radius: 0 }` — a
# rule for the tab strip the sidebar stopped being at ADR-018, still reaching the buttons that
# became its headers.
#
# **The group cards stay rounded under this patch and that is the point.** They carry no `.tab`
# class, so they take the button default of 6px; the failure names only the panel cards, which is
# exactly what shipped — two kinds of card in one column, one rounded and one not.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="have square corners"
