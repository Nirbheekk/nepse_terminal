import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/app/**/*.{ts,tsx}", "./src/components/**/*.{ts,tsx}", "./src/lib/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        // A dark, low-glare surface for long sessions; amber is reserved for
        // provenance/disclaimer text so it is never confused with price action.
        surface: {
          base: "#0b0e13",
          raised: "#12161d",
          line: "#1f2530",
          muted: "#8b95a6",
        },
        up: "#26a17b",
        down: "#d1495b",
        accent: "#d7a13b",
      },
      fontFamily: {
        mono: ["ui-monospace", "SFMono-Regular", "Menlo", "Consolas", "monospace"],
      },
      fontSize: {
        "2xs": ["0.6875rem", { lineHeight: "1rem" }],
      },
    },
  },
  plugins: [],
};

export default config;
