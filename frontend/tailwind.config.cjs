/** @type {import('tailwindcss').Config} */
export default {
  darkMode: ["class"],
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      /* ========== 道教色彩系统 ========== */
      colors: {
        cinnabar: {
          50: '#FDF2F0',
          100: '#FADBD6',
          200: '#F5B7AC',
          300: '#E88A7D',
          400: '#D85D52',
          500: '#C23A30',
          600: '#A82D26',
          700: '#8E2420',
          800: '#741D1A',
          900: '#5A1614',
        },
        gold: {
          50: '#FBF6E9',
          100: '#F3E7C0',
          200: '#E8D296',
          300: '#D4A843',
          400: '#C49A2A',
          500: '#B08A20',
          600: '#96741A',
          700: '#7C5E16',
          800: '#624812',
          900: '#4A360E',
        },
        ink: {
          50: '#F0F0F2',
          100: '#D9D9DE',
          200: '#B3B3BD',
          300: '#8D8D9C',
          400: '#67677B',
          500: '#4A4A5A',
          600: '#333340',
          700: '#1A1A2E',
          800: '#141424',
          900: '#0E0E1A',
        },
        'rice-paper': {
          50: '#FEFCF7',
          100: '#FDF9EF',
          200: '#FAF3DF',
          300: '#F5EAC8',
          400: '#F0E0B0',
          500: '#E8D49A',
          DEFAULT: '#F5F0E6',
        },
        jade: {
          50: '#E8F5F0',
          100: '#C1E8DA',
          200: '#9AD9C3',
          300: '#72C9AC',
          400: '#4BB896',
          500: '#2E8B76',
          600: '#257562',
          700: '#1D5F50',
          800: '#15493E',
          900: '#0E342C',
        },
        bronze: {
          50: '#F5EFE0',
          100: '#E6D6B8',
          200: '#D4BC8E',
          300: '#C2A266',
          400: '#AA8840',
          500: '#8B6914',
          600: '#755B10',
          700: '#5F4D0C',
          800: '#493F08',
          900: '#353104',
        },
      },
      /* ========== 自定义字体 ========== */
      fontFamily: {
        serif: ['"Noto Serif SC"', 'serif'],
        sans: ['"Noto Sans SC"', 'sans-serif'],
      },
      /* ========== 自定义动画 ========== */
      keyframes: {
        'float': {
          '0%, 100%': { transform: 'translateY(0)' },
          '50%': { transform: 'translateY(-10px)' },
        },
        'glow': {
          '0%, 100%': { opacity: '0.5', transform: 'scale(1)' },
          '50%': { opacity: '1', transform: 'scale(1.05)' },
        },
        'smoke': {
          '0%': { opacity: '0.8', transform: 'translateY(0) scale(1)' },
          '100%': { opacity: '0', transform: 'translateY(-60px) scale(1.5)' },
        },
        'spin-slow': {
          '0%': { transform: 'rotate(0deg)' },
          '100%': { transform: 'rotate(360deg)' },
        },
        'fade-in': {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        'typing': {
          '0%': { width: '0' },
          '100%': { width: '100%' },
        },
        'pulse-glow': {
          '0%, 100%': { boxShadow: '0 0 5px rgba(212, 168, 67, 0.3)' },
          '50%': { boxShadow: '0 0 20px rgba(212, 168, 67, 0.7)' },
        },
      },
      animation: {
        'float': 'float 3s ease-in-out infinite',
        'glow': 'glow 2s ease-in-out infinite',
        'smoke': 'smoke 2s ease-out infinite',
        'spin-slow': 'spin-slow 8s linear infinite',
        'fade-in': 'fade-in 0.3s ease-out',
        'typing': 'typing 1s steps(20, end)',
        'pulse-glow': 'pulse-glow 2s ease-in-out infinite',
      },
      /* ========== 背景纹理 ========== */
      backgroundImage: {
        'rice-paper': "url('data:image/svg+xml,%3Csvg width=\\'200\\' height=\\'200\\' viewBox=\\'0 0 200 200\\' xmlns=\\'http://www.w3.org/2000/svg\\'%3E%3Cfilter id=\\'noise\\'%3E%3CfeTurbulence type=\\'fractalNoise\\' baseFrequency=\\'0.65\\' numOctaves=\\'3\\' stitchTiles=\\'stitch\\'/%3E%3C/filter%3E%3Crect width=\\'200\\' height=\\'200\\' filter=\\'url(%23noise)\\' opacity=\\'0.05\\'/%3E%3C/svg%3E')",
      },
      borderRadius: {
        lg: 'var(--radius)',
        md: 'calc(var(--radius) - 2px)',
        sm: 'calc(var(--radius) - 4px)',
      },
    },
  },
  plugins: [
    require('@tailwindcss/typography'),
  ],
}
