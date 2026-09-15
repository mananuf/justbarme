import type { CSSProperties } from 'react';

// Inline mark from /logo.svg, using currentColor so it can sit on cream
// (dark ink) or on the dark green sections/footer (cream) without a second asset.
export function Logo({
  className = 'w-8 h-8',
  style,
}: {
  className?: string;
  style?: CSSProperties;
}) {
  return (
    <svg
      viewBox="0 0 1000 1000"
      fill="currentColor"
      className={className}
      style={style}
      aria-hidden="true"
    >
      <path
        d="M498.5,23.3C235.9,23.3,23,236.2,23,498.8v0C23,751.4,220,958,468.8,973.3v-202L108.2,425.5h768.7L524.3,770.8v202.8
        C774.9,960.2,974,752.7,974,498.8V23.3H498.5z M495.3,372.9C495.2,372.9,495.2,372.9,495.3,372.9
        C495.2,372.9,495.2,372.9,495.3,372.9L495.3,372.9c-218.1-26.9-191.9-216.5-191.6-218.6l0,0c0,0,0,0,0,0c0,0,0,0,0,0l0,0
        C521.7,181.2,495.5,370.7,495.3,372.9L495.3,372.9z M449.2,163.5c19.5-50.3,55.7-80.3,56.5-80.9l0,0c0,0,0,0,0,0c0,0,0,0,0,0l0,0
        c96.4,113.2,60,205.7,25.8,254C536.3,287.6,526.2,216.9,449.2,163.5z M696,164.6c-2.5,153-96,196.6-153.2,208.9
        c17.6-16.3,84.8-86.6,58.6-186.2C648.8,163.2,695,164.6,696,164.6L696,164.6C696,164.6,696,164.6,696,164.6
        C696,164.6,696.1,164.6,696,164.6L696,164.6z"
      />
    </svg>
  );
}
