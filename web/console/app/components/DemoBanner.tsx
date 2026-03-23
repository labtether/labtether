"use client";

export function DemoBanner() {
  if (process.env.NEXT_PUBLIC_DEMO_MODE !== "true") return null;

  return (
    <div className="bg-blue-600 text-white text-center text-sm py-2 px-4 shrink-0">
      You&apos;re exploring a demo instance with sample data.{" "}
      <a
        href="https://labtether.com/docs/getting-started/installation"
        className="underline font-medium hover:text-blue-100"
        target="_blank"
        rel="noopener noreferrer"
      >
        Install LabTether &rarr;
      </a>
    </div>
  );
}
