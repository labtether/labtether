/** While the Alerts page loads: an empty sheet where the list will be (no shimmer). */
export default function Loading() {
  return <div className="al-sheet al-empty" aria-busy="true" style={{ minHeight: 200 }} />;
}
