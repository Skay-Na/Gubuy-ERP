/** @type {import('tailwindcss').Config} */

// 全站品牌色：Stripe 紫 #635BFF。
// 把 Tailwind 的 blue / indigo 色板整体映射到品牌紫渐变梯，
// 使历史页面里所有 blue-* / indigo-* 工具类自动统一为新主色，无需逐个改类名。
const brandRamp = {
  50: '#eef0ff',
  100: '#e8e9ff',
  200: '#dcdaff',
  300: '#c0baff',
  400: '#9a91ff',
  500: '#7a70ff',
  600: '#635bff',
  700: '#4b44c8',
  800: '#3a35a0',
  900: '#2d2978',
  950: '#1e1b66'
}

export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        blue: brandRamp,
        indigo: brandRamp
      }
    },
  },
  plugins: [],
}
