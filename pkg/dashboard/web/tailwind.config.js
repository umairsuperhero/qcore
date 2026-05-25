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
