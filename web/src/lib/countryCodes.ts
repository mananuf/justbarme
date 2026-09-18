// Country dial codes for PhoneInput.tsx. Not the full ISO-3166 list --
// Nigeria first (the pilot market), then a broad but pragmatic set of
// other countries covering West Africa, the rest of Africa, and common
// diaspora/international destinations. Add to this list rather than
// building a second one if a new country is needed.
export interface Country {
  iso2: string;
  name: string;
  dialCode: string; // digits only, no '+'
}

export const COUNTRIES: Country[] = [
  { iso2: 'NG', name: 'Nigeria', dialCode: '234' },
  { iso2: 'GH', name: 'Ghana', dialCode: '233' },
  { iso2: 'CI', name: "Côte d'Ivoire", dialCode: '225' },
  { iso2: 'SN', name: 'Senegal', dialCode: '221' },
  { iso2: 'BJ', name: 'Benin', dialCode: '229' },
  { iso2: 'TG', name: 'Togo', dialCode: '228' },
  { iso2: 'CM', name: 'Cameroon', dialCode: '237' },
  { iso2: 'NE', name: 'Niger', dialCode: '227' },
  { iso2: 'ML', name: 'Mali', dialCode: '223' },
  { iso2: 'BF', name: 'Burkina Faso', dialCode: '226' },
  { iso2: 'GM', name: 'Gambia', dialCode: '220' },
  { iso2: 'SL', name: 'Sierra Leone', dialCode: '232' },
  { iso2: 'LR', name: 'Liberia', dialCode: '231' },
  { iso2: 'KE', name: 'Kenya', dialCode: '254' },
  { iso2: 'UG', name: 'Uganda', dialCode: '256' },
  { iso2: 'TZ', name: 'Tanzania', dialCode: '255' },
  { iso2: 'RW', name: 'Rwanda', dialCode: '250' },
  { iso2: 'ET', name: 'Ethiopia', dialCode: '251' },
  { iso2: 'ZA', name: 'South Africa', dialCode: '27' },
  { iso2: 'ZM', name: 'Zambia', dialCode: '260' },
  { iso2: 'ZW', name: 'Zimbabwe', dialCode: '263' },
  { iso2: 'EG', name: 'Egypt', dialCode: '20' },
  { iso2: 'MA', name: 'Morocco', dialCode: '212' },
  { iso2: 'DZ', name: 'Algeria', dialCode: '213' },
  { iso2: 'TN', name: 'Tunisia', dialCode: '216' },
  { iso2: 'US', name: 'United States', dialCode: '1' },
  { iso2: 'CA', name: 'Canada', dialCode: '1' },
  { iso2: 'GB', name: 'United Kingdom', dialCode: '44' },
  { iso2: 'IE', name: 'Ireland', dialCode: '353' },
  { iso2: 'FR', name: 'France', dialCode: '33' },
  { iso2: 'DE', name: 'Germany', dialCode: '49' },
  { iso2: 'ES', name: 'Spain', dialCode: '34' },
  { iso2: 'PT', name: 'Portugal', dialCode: '351' },
  { iso2: 'IT', name: 'Italy', dialCode: '39' },
  { iso2: 'NL', name: 'Netherlands', dialCode: '31' },
  { iso2: 'BE', name: 'Belgium', dialCode: '32' },
  { iso2: 'CH', name: 'Switzerland', dialCode: '41' },
  { iso2: 'SE', name: 'Sweden', dialCode: '46' },
  { iso2: 'NO', name: 'Norway', dialCode: '47' },
  { iso2: 'AE', name: 'United Arab Emirates', dialCode: '971' },
  { iso2: 'SA', name: 'Saudi Arabia', dialCode: '966' },
  { iso2: 'QA', name: 'Qatar', dialCode: '974' },
  { iso2: 'IN', name: 'India', dialCode: '91' },
  { iso2: 'PK', name: 'Pakistan', dialCode: '92' },
  { iso2: 'CN', name: 'China', dialCode: '86' },
  { iso2: 'JP', name: 'Japan', dialCode: '81' },
  { iso2: 'SG', name: 'Singapore', dialCode: '65' },
  { iso2: 'MY', name: 'Malaysia', dialCode: '60' },
  { iso2: 'AU', name: 'Australia', dialCode: '61' },
  { iso2: 'BR', name: 'Brazil', dialCode: '55' },
];

// Regional indicator letters (used as a flag emoji) sit at a fixed offset
// from ASCII 'A'-'Z' in Unicode -- computing it avoids hand-typing (and
// mistyping) 50 separate flag glyphs.
const REGIONAL_INDICATOR_OFFSET = 127397;

export function countryFlagEmoji(iso2: string): string {
  return iso2
    .toUpperCase()
    .replace(/./g, (char) => String.fromCodePoint(REGIONAL_INDICATOR_OFFSET + char.charCodeAt(0)));
}

// Strips everything but digits, then drops a single leading local-dialing
// '0' (e.g. Nigerian numbers are conventionally written "0803 123 4567")
// before it's prefixed with a country dial code -- E.164 never has one.
export function normalizeLocalNumber(raw: string): string {
  const digits = raw.replace(/\D/g, '');
  return digits.startsWith('0') ? digits.slice(1) : digits;
}

export function toE164(dialCode: string, localNumber: string): string {
  const normalized = normalizeLocalNumber(localNumber);
  return normalized ? `+${dialCode}${normalized}` : '';
}
