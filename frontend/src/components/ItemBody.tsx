import { Box } from "@mui/material";

// An external-link icon (Material's OpenInNew) drawn after links that
// leave the app, as a mask so it takes the link's color.
const externalIcon = `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'%3E%3Cpath d='M19 19H5V5h7V3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14c1.1 0 2-.9 2-2v-7h-2zM14 3v2h3.59l-9.83 9.83 1.41 1.41L19 6.41V10h2V3z'/%3E%3C/svg%3E")`;

// ItemBody renders an item's body HTML. The server sanitizes it
// (bluemonday) and points every link at pingry.org or beyond, opening in a
// new tab.
export default function ItemBody({ html }: { html: string }) {
  return (
    <Box
      sx={{
        overflowWrap: "anywhere",
        "& p": { my: 0, mb: 1.25 },
        "& ul, & ol": { pl: 3, my: 0, mb: 1.25 },
        "& li": { mb: 0.5 },
        "& img": { maxWidth: "100%", height: "auto" },
        "& table": { display: "block", overflowX: "auto", maxWidth: "100%" },
        "& a": {
          color: "blue.main",
          textUnderlineOffset: "0.15em",
        },
        "& a[target=_blank]::after": {
          content: '""',
          display: "inline-block",
          width: "0.85em",
          height: "0.85em",
          ml: 0.25,
          verticalAlign: "-0.05em",
          bgcolor: "currentColor",
          maskImage: externalIcon,
          maskSize: "contain",
        },
        "& > :last-child": { mb: 0 },
      }}
      // Sanitized server-side; see handler/html.go.
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
