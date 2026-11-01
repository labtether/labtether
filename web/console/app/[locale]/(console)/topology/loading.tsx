/** While the Map's code loads: a quiet, empty stage (the page draws its own title). */
export default function Loading() {
  return <div className="mp-quiet" role="status" aria-busy="true" />;
}
