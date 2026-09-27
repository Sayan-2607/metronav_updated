import type { Config } from "tailwindcss";

// Palette taken from the metro's own vocabulary: platform concrete, signage ink,
// and the three line colours. Occupancy uses its own ramp so it never reads as a line.
const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        concrete: "#E6E9EC",
        panel: "#FBFCFD",
        ink: "#16202B",
        mute: "#5B6875",
        rule: "#C9D0D7",
        blue: { line: "#1F5FAE" },
        green: { line: "#1E8F4E" },
        yellow: { line: "#D9A400" },
        occ: { low: "#7FB8A4", mid: "#E3B448", high: "#E07B39", severe: "#C8352E" },
      },
      fontFamily: {
        sans: ['"Barlow"', "system-ui", "Segoe UI", "Roboto", "sans-serif"],
        sign: ['"Barlow Condensed"', '"Arial Narrow"', "system-ui", "sans-serif"],
      },
    },
  },
  plugins: [],
};
export default config;
