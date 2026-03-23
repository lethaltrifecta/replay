import { ExperimentReportScreen } from "@/components/experiment-report-screen";

export default async function GauntletDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  return <ExperimentReportScreen experimentId={id} />;
}
