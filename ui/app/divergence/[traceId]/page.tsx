import { DriftDetailScreen } from "@/components/drift-detail-screen";

export default async function DivergenceDetailPage({
  params,
  searchParams,
}: {
  params: Promise<{ traceId: string }>;
  searchParams: Promise<{ baseline?: string }>;
}) {
  const [{ traceId }, { baseline }] = await Promise.all([params, searchParams]);

  return <DriftDetailScreen traceId={traceId} baselineTraceId={baseline} />;
}
