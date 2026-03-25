import { CompareScreen } from "@/components/compare-screen";

export default function ShadowReplayPage({
  searchParams,
}: {
  searchParams: Promise<{ baseline?: string; candidate?: string }>;
}) {
  return (
    <div className="flex flex-col gap-6">
      <ShadowReplayContent searchParams={searchParams} />
    </div>
  );
}

async function ShadowReplayContent({
  searchParams,
}: {
  searchParams: Promise<{ baseline?: string; candidate?: string }>;
}) {
  const { baseline, candidate } = await searchParams;

  return (
    <CompareScreen
      baselineTraceId={baseline}
      candidateTraceId={candidate}
    />
  );
}
