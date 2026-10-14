import { createTheme } from "@mui/material/styles";

// Tokens from docs/DESIGN.md, "Visual design". A school bulletin board:
// Ink on Paper, Big Blue for what's new, Struck for corrections and close
// deadlines, and Flyer yellow reserved for signup items, where the
// different surface itself carries meaning.
const tokens = {
  light: {
    paper: "#F6F8FB",
    ink: "#0B2A5B",
    inkSoft: "#4A5B78",
    blue: "#1F5FD6",
    struck: "#B23A2E",
    flyer: "#FFF1A8",
    flyerEdge: "#D9C766",
    rule: "#C8D2E2",
  },
  dark: {
    paper: "#0F1B2E",
    ink: "#E4EAF5",
    inkSoft: "#A9B6CC",
    blue: "#7FA8F5",
    struck: "#F08A7E",
    flyer: "#3A3410",
    flyerEdge: "#6B6024",
    rule: "#2A3A55",
  },
} as const;

declare module "@mui/material/styles" {
  interface Palette {
    blue: Palette["primary"];
    struck: Palette["primary"];
    flyer: Palette["primary"];
  }
  interface PaletteOptions {
    blue?: PaletteOptions["primary"];
    struck?: PaletteOptions["primary"];
    flyer?: PaletteOptions["primary"];
  }
}

const font = "'Atkinson Hyperlegible Next', system-ui, sans-serif";

function scheme(t: (typeof tokens)["light" | "dark"]) {
  return {
    palette: {
      primary: { main: t.ink, contrastText: t.paper },
      blue: { main: t.blue },
      struck: { main: t.struck },
      // dark is the perforation and tab rules on a flyer.
      flyer: { main: t.flyer, dark: t.flyerEdge, contrastText: t.ink },
      error: { main: t.struck },
      background: { default: t.paper, paper: t.paper },
      text: { primary: t.ink, secondary: t.inkSoft },
      divider: t.rule,
    },
  };
}

const theme = createTheme({
  cssVariables: { colorSchemeSelector: "media" },
  colorSchemes: { light: scheme(tokens.light), dark: scheme(tokens.dark) },
  shape: { borderRadius: 4 },
  typography: {
    fontFamily: font,
    fontSize: 15,
    body1: { fontSize: "0.9375rem", lineHeight: 1.5 },
    body2: { fontSize: "0.8125rem", lineHeight: 17 / 13 },
    // Item titles.
    h3: { fontSize: "1.0625rem", lineHeight: 22 / 17, fontWeight: 700 },
    // Masthead.
    h1: { fontSize: "1.625rem", lineHeight: 30 / 26, fontWeight: 800 },
    h2: { fontSize: "1.0625rem", lineHeight: 22 / 17, fontWeight: 700 },
    button: { textTransform: "none", fontWeight: 700, fontSize: "0.9375rem" },
  },
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        body: { fontVariantNumeric: "tabular-nums" },
        ":focus-visible": {
          outline: `2px solid var(--mui-palette-blue-main)`,
          outlineOffset: 2,
        },
        a: { color: "inherit" },
        "@media (prefers-reduced-motion: reduce)": {
          "*, *::before, *::after": {
            transitionDuration: "0.01ms !important",
            animationDuration: "0.01ms !important",
          },
        },
      },
    },
    // No elevation tint in dark mode: surfaces stay Paper.
    MuiPaper: { styleOverrides: { root: { backgroundImage: "none" } } },
    MuiButtonBase: { defaultProps: { disableRipple: true } },
    MuiButton: {
      defaultProps: { disableElevation: true },
      styleOverrides: { root: { minHeight: 44 } },
    },
    MuiIconButton: {
      styleOverrides: { root: { minWidth: 44, minHeight: 44 } },
    },
    MuiTab: {
      styleOverrides: {
        root: {
          minHeight: 44,
          minWidth: 0,
          paddingInline: 12,
          textTransform: "none",
          fontWeight: 700,
          fontSize: "0.9375rem",
        },
      },
    },
  },
});

export default theme;
