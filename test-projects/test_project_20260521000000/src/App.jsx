import { useEffect, useMemo, useState } from 'react';

const zones = [
  { label: 'Local', value: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC' },
  { label: 'New York', value: 'America/New_York' },
  { label: 'Los Angeles', value: 'America/Los_Angeles' },
  { label: 'London', value: 'Europe/London' },
  { label: 'Tokyo', value: 'Asia/Tokyo' },
  { label: 'Sydney', value: 'Australia/Sydney' },
  { label: 'UTC', value: 'UTC' },
];

function formatTime(date, timeZone) {
  return new Intl.DateTimeFormat('en-US', {
    timeZone,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date);
}

function formatDate(date, timeZone) {
  return new Intl.DateTimeFormat('en-US', {
    timeZone,
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  }).format(date);
}

export default function App() {
  const [now, setNow] = useState(() => new Date());
  const [timeZone, setTimeZone] = useState(zones[0].value);

  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), 1000);
    return () => window.clearInterval(id);
  }, []);

  const activeZone = useMemo(() => zones.find((zone) => zone.value === timeZone) || zones[0], [timeZone]);

  return (
    <main className="min-h-screen bg-slate-950 text-slate-100">
      <section className="mx-auto flex min-h-screen w-full max-w-3xl flex-col justify-center px-6 py-10">
        <div className="rounded-lg border border-slate-700 bg-slate-900 p-6 shadow-2xl shadow-black/30">
          <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <p className="text-sm font-semibold uppercase tracking-wide text-cyan-300">Omnidex Clock</p>
              <h1 className="mt-2 text-3xl font-bold text-white">World Clock</h1>
            </div>
            <label className="flex flex-col gap-2 text-sm font-medium text-slate-300">
              Timezone
              <select
                className="rounded-md border border-slate-600 bg-slate-800 px-3 py-2 text-white outline-none ring-cyan-400 transition focus:ring-2"
                value={timeZone}
                onChange={(event) => setTimeZone(event.target.value)}
              >
                {zones.map((zone) => (
                  <option key={zone.label} value={zone.value}>{zone.label}</option>
                ))}
              </select>
            </label>
          </div>

          <div className="rounded-lg bg-slate-950 p-6">
            <p className="text-sm font-medium text-slate-400">{activeZone.label} · {timeZone}</p>
            <time className="mt-3 block font-mono text-5xl font-bold tabular-nums text-cyan-200 sm:text-7xl">
              {formatTime(now, timeZone)}
            </time>
            <p className="mt-4 text-lg text-slate-300">{formatDate(now, timeZone)}</p>
          </div>
        </div>
      </section>
    </main>
  );
}
