// The Document language field's pre-fill — `/pending 471`, Dan's option B: the field starts at this
// computer's language, and the user can change or clear it.
//
// A module of its own, like detect.js, so the choice is a pure function a test can table rather than
// something only observable through one boot with one navigator.language.

// docLangForLocale picks the field's option for a BCP 47 locale. `values` are the options' values;
// the empty value means "not specified".
//
// Exact tag first (case-insensitive), then the primary subtag, else "" — an unmapped locale declares
// nothing rather than a near guess. Chinese is the one language whose primary subtag does not pick
// an option: the options are scripts, and the region is what says which one a reader uses.
export function docLangForLocale(locale, values) {
  if (!locale) return '';
  const byLower = new Map(values.filter(Boolean).map((v) => [v.toLowerCase(), v]));
  const want = String(locale).toLowerCase().replace(/_/g, '-');
  if (byLower.has(want)) return byLower.get(want);
  const parts = want.split('-');
  if (parts[0] === 'zh') {
    const traditional = parts.includes('hant') || parts.some((p) => ['tw', 'hk', 'mo'].includes(p));
    return byLower.get(traditional ? 'zh-hant' : 'zh-hans') || '';
  }
  return byLower.get(parts[0]) || '';
}
