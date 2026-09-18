import { useId, useState } from 'react';

import { COUNTRIES, countryFlagEmoji, toE164 } from '../lib/countryCodes';

interface PhoneInputProps {
  // The composed E.164 number ("" until a local number has been typed).
  onChange: (e164: string) => void;
  required?: boolean;
  autoFocus?: boolean;
  placeholder?: string;
  // Defaults to Nigeria ('NG') -- the pilot market.
  defaultCountryIso2?: string;
}

// A country-code + local-number phone entry, styled as one field. Always
// composes to a real E.164 string (see src/lib/countryCodes.ts) -- this is
// what actually fixed the "zavu returned 400: Numeric chat IDs can only be
// used with..." bug: a bare local number like "08031234567" was never
// valid E.164 to begin with, this component is what makes that
// unrepresentable rather than relying on the user to type "+234" correctly.
export function PhoneInput({
  onChange,
  required,
  autoFocus,
  placeholder = '803 123 4567',
  defaultCountryIso2 = 'NG',
}: PhoneInputProps) {
  const selectId = useId();
  const [iso2, setIso2] = useState(defaultCountryIso2);
  const [local, setLocal] = useState('');

  const country = COUNTRIES.find((c) => c.iso2 === iso2) ?? COUNTRIES[0]!;

  function emit(nextIso2: string, nextLocal: string) {
    const dialCode = COUNTRIES.find((c) => c.iso2 === nextIso2)?.dialCode ?? country.dialCode;
    onChange(toE164(dialCode, nextLocal));
  }

  return (
    <div className="flex rounded-xl border border-jb-ink/15 bg-white focus-within:border-jb-ink/40 transition-colors overflow-hidden">
      <label htmlFor={selectId} className="sr-only">
        Country code
      </label>
      <select
        id={selectId}
        value={iso2}
        onChange={(e) => {
          setIso2(e.target.value);
          emit(e.target.value, local);
        }}
        aria-label="Country code"
        className="shrink-0 bg-transparent pl-3.5 pr-1.5 py-3.5 text-[15px] text-jb-ink border-r border-jb-ink/10 focus:outline-none appearance-none"
      >
        {COUNTRIES.map((c) => (
          <option key={c.iso2} value={c.iso2}>
            {countryFlagEmoji(c.iso2)} +{c.dialCode}
          </option>
        ))}
      </select>
      <input
        type="tel"
        inputMode="tel"
        autoComplete="tel-national"
        required={required}
        autoFocus={autoFocus}
        placeholder={placeholder}
        value={local}
        onChange={(e) => {
          setLocal(e.target.value);
          emit(iso2, e.target.value);
        }}
        className="min-w-0 flex-1 px-3.5 py-3.5 text-[15px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none"
      />
    </div>
  );
}
