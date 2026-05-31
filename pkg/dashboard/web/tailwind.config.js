/** @type {import('tailwindcss').Config} */
export default {
  content: ["./src/**/*.{ts,tsx,html}"],
  theme: {
    extend: {
      colors: {
        // QCore brand colors — calm, technical, not playful
        slateblue: {
          50: "#f5f7fb",
          100: "#e6ebf5",
          500: "#5b6b8c",
          700: "#3b4660",
          900: "#1f2638",
        },
        darkbg: {
          950: "#0b0f19",
          900: "#131a26",
          800: "#1b2536",
          700: "#222f43",
          600: "#2d3e56",
        },
      },
      fontFamily: {
        mono: [
          "ui-monospace",
          "SFMono-Regular",
          "Menlo",
          "Monaco",
          "Consolas",
          "monospace",
        ],
      },
    },
  },
  plugins: [],
};
