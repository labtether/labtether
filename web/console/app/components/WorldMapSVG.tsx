"use client";

/**
 * Lightweight world map outline for the reliability map tab.
 * Uses a simplified equirectangular projection (Plate Carrée).
 * Continent paths are heavily simplified to keep the bundle small.
 */
export function WorldMapSVG({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 360 180"
      preserveAspectRatio="none"
      className={className}
      aria-hidden="true"
    >
      {/* Graticule grid lines every 30° */}
      <g stroke="var(--line)" strokeWidth="0.3" fill="none" opacity="0.5">
        {[30, 60, 90, 120, 150, 210, 240, 270, 300, 330].map((x) => (
          <line key={`v${x}`} x1={x} y1={0} x2={x} y2={180} />
        ))}
        {[30, 60, 120, 150].map((y) => (
          <line key={`h${y}`} x1={0} y1={y} x2={360} y2={y} />
        ))}
        {/* Equator */}
        <line x1={0} y1={90} x2={360} y2={90} strokeDasharray="2 2" opacity="0.7" />
      </g>

      {/* Simplified continent outlines — lon shifted +180 so 0°lon = x:180 */}
      <g
        fill="var(--text)"
        fillOpacity="0.06"
        stroke="var(--text)"
        strokeOpacity="0.15"
        strokeWidth="0.5"
      >
        {/* North America */}
        <path d="M40,25 L55,22 L70,25 L80,30 L85,40 L80,50 L75,55 L70,58 L60,60 L55,65 L50,70 L45,72 L40,68 L30,60 L25,50 L28,40 L35,30 Z" />
        {/* Central America & Caribbean */}
        <path d="M55,65 L60,68 L65,72 L68,75 L65,78 L60,80 L55,78 L50,75 L50,72 Z" />
        {/* South America */}
        <path d="M65,78 L70,82 L75,88 L78,95 L80,105 L78,115 L75,125 L70,135 L65,140 L60,138 L55,130 L52,120 L50,110 L50,100 L52,92 L55,85 L58,80 Z" />
        {/* Europe */}
        <path d="M165,30 L170,28 L180,25 L190,28 L195,32 L200,35 L198,40 L195,42 L190,45 L185,48 L180,50 L175,48 L170,45 L165,42 L162,38 L163,34 Z" />
        {/* Africa */}
        <path d="M170,50 L175,48 L180,50 L185,52 L195,55 L200,60 L205,70 L208,80 L210,90 L208,100 L205,110 L200,118 L195,122 L190,120 L185,115 L180,108 L175,100 L170,90 L168,80 L165,70 L165,60 Z" />
        {/* Asia */}
        <path d="M195,28 L210,22 L225,18 L240,15 L255,18 L270,20 L285,25 L295,30 L305,28 L310,35 L305,40 L295,42 L285,45 L275,48 L265,50 L255,52 L250,55 L245,60 L240,65 L235,62 L230,58 L225,55 L218,52 L210,48 L205,45 L200,40 L198,35 Z" />
        {/* India */}
        <path d="M245,60 L250,58 L255,62 L258,68 L260,75 L258,82 L255,86 L250,84 L245,78 L242,72 L242,66 Z" />
        {/* Southeast Asia */}
        <path d="M265,55 L275,52 L280,58 L285,62 L290,68 L288,72 L282,70 L278,65 L272,62 L268,58 Z" />
        {/* Australia */}
        <path d="M285,105 L295,100 L305,98 L315,100 L320,105 L322,112 L320,120 L315,125 L305,128 L295,125 L288,120 L285,112 Z" />
        {/* Greenland */}
        <path d="M100,18 L110,15 L118,18 L120,24 L115,28 L108,28 L102,25 Z" />
      </g>
    </svg>
  );
}
