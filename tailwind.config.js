/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./components/**/*.templ"],
  theme: {
    extend: {},
  },
  plugins: [require("daisyui")],
  daisyui: {
    themes: ["light", "dark", "cupcake"], // Add any themes you want to use
  },
}
