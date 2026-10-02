"use client";

export default function ConsoleError({ reset }: { reset: () => void }) {
  return <><h1>The console could not load this page</h1>
    <p className="notice error" role="alert">An unexpected response interrupted the page. Retry after checking the API and console logs.</p>
    <button onClick={reset}>Retry</button>
  </>;
}
