export type IconName = "overview" | "users" | "downloads" | "crashes" | "skins" | "dictionaries" | "replies" | "audit" | "refresh" | "arrow-right";

const paths: Record<IconName, string> = {
  overview: "M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z",
  users: "M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2 M16 3a4 4 0 0 1 0 8 M22 21v-2a4 4 0 0 0-3-3.87 M13 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0",
  downloads: "M12 3v12 M7 10l5 5 5-5 M4 16v5h16v-5",
  crashes: "M12 8v5 M12 17h.01 M10.3 3.9 2 18.3A1.8 1.8 0 0 0 3.6 21h16.8a1.8 1.8 0 0 0 1.6-2.7L13.7 3.9a2 2 0 0 0-3.4 0Z",
  skins: "m8 3-6 4 3 5 3-2v11h8V10l3 2 3-5-6-4a4 4 0 0 1-8 0Z",
  dictionaries: "M12 5v16 M3 3h5a4 4 0 0 1 4 2 4 4 0 0 1 4-2h5v16h-5a4 4 0 0 0-4 2 4 4 0 0 0-4-2H3Z",
  replies: "M21 11a8 8 0 0 1-8 8H8l-5 3V5a2 2 0 0 1 2-2h8a8 8 0 0 1 8 8Z M7 8h10 M7 13h6",
  audit: "M21 12a9 9 0 1 1-2.6-6.4 M21 3v6h-6 M12 7v5l3 2",
  refresh: "M20 7a9 9 0 0 0-15-2L2 8 M2 2v6h6 M4 17a9 9 0 0 0 15 2l3-3 M22 22v-6h-6",
  "arrow-right": "M4 12h16 M14 6l6 6-6 6",
};

export function Icon({ name, className }: { name: IconName; className?: string }) {
  return <svg className={className} width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false"><path d={paths[name]} /></svg>;
}
