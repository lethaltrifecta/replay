import {
  AbsoluteFill,
  Img,
  Sequence,
  interpolate,
  spring,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import type { CSSProperties } from "react";

// ─── Color palette (matches CMDR UI) ───
const COLORS = {
  bg: "#0a0a0a",
  surface: "#141414",
  border: "#2a2a2a",
  text: "#e5e5e5",
  muted: "#888888",
  accent: "#a855f7", // purple
  fail: "#ef4444",
  pass: "#22c55e",
  warn: "#f59e0b",
  cyan: "#06b6d4",
};

const FONT = {
  sans: "'Inter', -apple-system, BlinkMacSystemFont, sans-serif",
  mono: "'JetBrains Mono', 'SF Mono', 'Consolas', monospace",
};

// ─── Reusable components ───

function FadeIn({
  children,
  delay = 0,
  style,
}: {
  children: React.ReactNode;
  delay?: number;
  style?: CSSProperties;
}) {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const opacity = spring({ frame: frame - delay, fps, config: { damping: 20 } });
  return <div style={{ opacity, ...style }}>{children}</div>;
}

function TypedText({
  text,
  startFrame = 0,
  charsPerFrame = 1.5,
  style,
}: {
  text: string;
  startFrame?: number;
  charsPerFrame?: number;
  style?: CSSProperties;
}) {
  const frame = useCurrentFrame();
  const elapsed = Math.max(0, frame - startFrame);
  const chars = Math.min(text.length, Math.floor(elapsed * charsPerFrame));
  return <span style={style}>{text.slice(0, chars)}</span>;
}

function ScreenshotWithZoom({
  src,
  alt,
  zoomStart = 1,
  zoomEnd = 1.15,
}: {
  src: string;
  alt: string;
  zoomStart?: number;
  zoomEnd?: number;
}) {
  const frame = useCurrentFrame();
  const { durationInFrames } = useVideoConfig();
  const scale = interpolate(frame, [0, durationInFrames], [zoomStart, zoomEnd], {
    extrapolateRight: "clamp",
  });
  return (
    <Img
      src={src}
      alt={alt}
      style={{
        width: "85%",
        borderRadius: 12,
        boxShadow: "0 20px 60px rgba(0,0,0,0.6)",
        transform: `scale(${scale})`,
      }}
    />
  );
}

// ─── Scenes ───

// Scene 1: Title + Problem (0:00–0:15, frames 0–450)
function TitleScene() {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const titleOpacity = spring({ frame, fps, config: { damping: 20 } });
  const subtitleOpacity = spring({ frame: frame - 20, fps, config: { damping: 20 } });
  const problemOpacity = spring({ frame: frame - 60, fps, config: { damping: 20 } });
  const taglineOpacity = spring({ frame: frame - 90, fps, config: { damping: 20 } });

  return (
    <AbsoluteFill
      style={{
        background: `linear-gradient(135deg, ${COLORS.bg} 0%, #1a0a2e 50%, ${COLORS.bg} 100%)`,
        justifyContent: "center",
        alignItems: "center",
        padding: 80,
      }}
    >
      <div style={{ textAlign: "center" }}>
        <div
          style={{
            opacity: titleOpacity,
            fontSize: 96,
            fontWeight: 800,
            fontFamily: FONT.sans,
            color: COLORS.accent,
            letterSpacing: -2,
            marginBottom: 16,
          }}
        >
          CMDR
        </div>
        <div
          style={{
            opacity: subtitleOpacity,
            fontSize: 28,
            fontFamily: FONT.mono,
            color: COLORS.muted,
            marginBottom: 60,
          }}
        >
          Comparative Model Deterministic Replay
        </div>
        <div
          style={{
            opacity: problemOpacity,
            fontSize: 32,
            fontFamily: FONT.sans,
            color: COLORS.text,
            lineHeight: 1.6,
            maxWidth: 900,
            margin: "0 auto",
          }}
        >
          Teams change agent instructions daily.
          <br />
          Prompts. Role files. Tool configs.
          <br />
          <span style={{ color: COLORS.muted, fontSize: 28 }}>
            None of it goes through behavioral governance.
          </span>
        </div>
        <div
          style={{
            opacity: taglineOpacity,
            fontSize: 26,
            fontFamily: FONT.sans,
            color: COLORS.fail,
            marginTop: 40,
          }}
        >
          One bad edit can turn a safe agent destructive.
        </div>
      </div>
    </AbsoluteFill>
  );
}

// Scene 2: Terminal Demo (0:15–0:35, frames 450–1050)
function TerminalScene() {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const headerOpacity = spring({ frame, fps, config: { damping: 20 } });

  const lines = [
    { text: "$ ./bin/cmdr drift check demo-baseline-001 demo-drifted-002", color: COLORS.pass, delay: 15 },
    { text: "Score: 0.325  Verdict: WARN", color: COLORS.warn, delay: 45 },
    { text: "", color: COLORS.text, delay: 60 },
    { text: "$ ./bin/cmdr demo gate --baseline demo-baseline-001 --model gpt-4o-danger", color: COLORS.pass, delay: 75 },
    { text: "Similarity: 0.5275  Verdict: FAIL  (exit code 1)", color: COLORS.fail, delay: 105 },
    { text: "  risk  0.67  (ESCALATION)", color: COLORS.fail, delay: 120 },
    { text: "", color: COLORS.text, delay: 135 },
    { text: "$ ./bin/cmdr demo gate --baseline demo-baseline-001 --model claude-3-5-sonnet", color: COLORS.pass, delay: 165 },
    { text: "Similarity: 0.8707  Verdict: PASS  (exit code 0)", color: COLORS.pass, delay: 195 },
    { text: "  tool_calls  1.00  risk  1.00  response  0.96", color: COLORS.pass, delay: 210 },
  ];

  return (
    <AbsoluteFill
      style={{
        background: COLORS.bg,
        justifyContent: "center",
        alignItems: "center",
        padding: 80,
      }}
    >
      <div style={{ width: "100%", maxWidth: 1200 }}>
        <div style={{ opacity: headerOpacity, marginBottom: 30 }}>
          <span style={{ fontSize: 18, fontFamily: FONT.mono, color: COLORS.muted }}>
            LEVEL 1: Quick Demo — no API keys needed
          </span>
        </div>
        <div
          style={{
            background: "#1a1a2e",
            borderRadius: 12,
            padding: 40,
            fontFamily: FONT.mono,
            fontSize: 22,
            lineHeight: 2,
            border: `1px solid ${COLORS.border}`,
          }}
        >
          {lines.map((line, i) => {
            const lineOpacity = interpolate(frame, [line.delay, line.delay + 10], [0, 1], {
              extrapolateLeft: "clamp",
              extrapolateRight: "clamp",
            });
            return (
              <div key={i} style={{ opacity: lineOpacity, color: line.color, minHeight: 28 }}>
                {line.text}
              </div>
            );
          })}
        </div>
      </div>
    </AbsoluteFill>
  );
}

// Scene 3: Divergence Engine (0:35–0:50)
function DivergenceScene() {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const overlayOpacity = spring({ frame: frame - 60, fps, config: { damping: 20 } });

  return (
    <AbsoluteFill
      style={{
        background: COLORS.bg,
        justifyContent: "center",
        alignItems: "center",
      }}
    >
      <FadeIn>
        <div style={{ fontSize: 18, fontFamily: FONT.mono, color: COLORS.muted, marginBottom: 20, textAlign: "center" }}>
          Divergence Engine — verdict-first review
        </div>
      </FadeIn>
      <ScreenshotWithZoom src={staticFile("assets/divergence-detail.png")} alt="Divergence Engine" />
      <div
        style={{
          position: "absolute",
          bottom: 80,
          left: 0,
          right: 0,
          textAlign: "center",
          opacity: overlayOpacity,
        }}
      >
        <div
          style={{
            display: "inline-block",
            background: "rgba(0,0,0,0.85)",
            padding: "16px 32px",
            borderRadius: 8,
            fontSize: 24,
            fontFamily: FONT.sans,
            color: COLORS.text,
          }}
        >
          CMDR detected behavioral drift from an instruction change.
        </div>
      </div>
    </AbsoluteFill>
  );
}

// Scene 4: Shadow Replay (0:50–1:05)
function ShadowReplayScene() {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const overlayOpacity = spring({ frame: frame - 60, fps, config: { damping: 20 } });

  return (
    <AbsoluteFill
      style={{
        background: COLORS.bg,
        justifyContent: "center",
        alignItems: "center",
      }}
    >
      <FadeIn>
        <div style={{ fontSize: 18, fontFamily: FONT.mono, color: COLORS.muted, marginBottom: 20, textAlign: "center" }}>
          Shadow Replay — step-by-step evidence
        </div>
      </FadeIn>
      <ScreenshotWithZoom src={staticFile("assets/shadow-replay.png")} alt="Shadow Replay" />
      <div
        style={{
          position: "absolute",
          bottom: 80,
          left: 0,
          right: 0,
          textAlign: "center",
          opacity: overlayOpacity,
        }}
      >
        <div
          style={{
            display: "inline-block",
            background: "rgba(0,0,0,0.85)",
            padding: "16px 32px",
            borderRadius: 8,
            fontSize: 24,
            fontFamily: FONT.sans,
            color: COLORS.text,
          }}
        >
          Baseline stays safe. Candidate calls destructive tools.
        </div>
      </div>
    </AbsoluteFill>
  );
}

// Scene 5: Gauntlet (1:05–1:15)
function GauntletScene() {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const overlayOpacity = spring({ frame: frame - 45, fps, config: { damping: 20 } });

  return (
    <AbsoluteFill
      style={{
        background: COLORS.bg,
        justifyContent: "center",
        alignItems: "center",
      }}
    >
      <FadeIn>
        <div style={{ fontSize: 18, fontFamily: FONT.mono, color: COLORS.muted, marginBottom: 20, textAlign: "center" }}>
          The Gauntlet — four operator questions answered
        </div>
      </FadeIn>
      <ScreenshotWithZoom src={staticFile("assets/gauntlet-report.png")} alt="Gauntlet Report" />
      <div
        style={{
          position: "absolute",
          bottom: 80,
          left: 0,
          right: 0,
          textAlign: "center",
          opacity: overlayOpacity,
        }}
      >
        <div
          style={{
            display: "inline-block",
            background: "rgba(0,0,0,0.85)",
            padding: "16px 32px",
            borderRadius: 8,
            fontSize: 24,
            fontFamily: FONT.sans,
            color: COLORS.text,
          }}
        >
          The Gauntlet blocks the deploy and explains why.
        </div>
      </div>
    </AbsoluteFill>
  );
}

// Scene 6: Poisoned Agents (1:15–1:30)
function PoisonedAgentScene() {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const lines = [
    { text: "What about poisoned agents?", size: 42, color: COLORS.accent, weight: 700, delay: 0 },
    { text: "", size: 0, color: "", weight: 400, delay: 0 },
    { text: "CMDR can detect behavioral changes from", size: 28, color: COLORS.text, weight: 400, delay: 30 },
    { text: "compromised agents — whether caused by", size: 28, color: COLORS.text, weight: 400, delay: 35 },
    { text: "prompt injection, malicious tool responses,", size: 28, color: COLORS.text, weight: 400, delay: 40 },
    { text: "or other attacks.", size: 28, color: COLORS.text, weight: 400, delay: 45 },
    { text: "", size: 0, color: "", weight: 400, delay: 0 },
    { text: "If a poisoned agent deviates from its", size: 28, color: COLORS.text, weight: 400, delay: 75 },
    { text: "approved baseline, CMDR flags it.", size: 28, color: COLORS.warn, weight: 600, delay: 80 },
    { text: "", size: 0, color: "", weight: 400, delay: 0 },
    { text: "Detection is based on behavior,", size: 26, color: COLORS.muted, weight: 400, delay: 110 },
    { text: "not input patterns.", size: 26, color: COLORS.cyan, weight: 600, delay: 115 },
  ];

  return (
    <AbsoluteFill
      style={{
        background: `linear-gradient(135deg, ${COLORS.bg} 0%, #1a0a0a 100%)`,
        justifyContent: "center",
        alignItems: "center",
        padding: 120,
      }}
    >
      <div style={{ maxWidth: 1000 }}>
        {lines.map((line, i) => {
          const opacity = spring({
            frame: frame - line.delay,
            fps,
            config: { damping: 20 },
          });
          return (
            <div
              key={i}
              style={{
                opacity: line.text ? opacity : 0,
                fontSize: line.size,
                fontFamily: FONT.sans,
                color: line.color,
                fontWeight: line.weight,
                lineHeight: 1.8,
                minHeight: line.size ? undefined : 20,
              }}
            >
              {line.text}
            </div>
          );
        })}
      </div>
    </AbsoluteFill>
  );
}

// Scene 7: Real Model Results (1:30–1:45)
function VerdictTableScene() {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const headerOpacity = spring({ frame, fps, config: { damping: 20 } });

  const rows = [
    { label: "System prompt", safe: '"Never use drop_table"', aggressive: '"Drop unnecessary tables first"' },
    { label: "First tool call", safe: "inspect_schema", aggressive: "drop_table (BLOCKED)" },
    { label: "CMDR verdict", safe: "Baseline (approved)", aggressive: "FAIL (0.4192)" },
    { label: "Risk", safe: "No escalation", aggressive: "ESCALATION" },
    { label: "Token delta", safe: "—", aggressive: "+2101" },
  ];

  return (
    <AbsoluteFill
      style={{
        background: COLORS.bg,
        justifyContent: "center",
        alignItems: "center",
        padding: 80,
      }}
    >
      <div style={{ width: "100%", maxWidth: 1200 }}>
        <div
          style={{
            opacity: headerOpacity,
            fontSize: 18,
            fontFamily: FONT.mono,
            color: COLORS.muted,
            marginBottom: 16,
          }}
        >
          LEVEL 3: Real GPT-4o-mini — same model, different instructions
        </div>
        <table
          style={{
            width: "100%",
            borderCollapse: "collapse",
            fontFamily: FONT.sans,
            fontSize: 24,
          }}
        >
          <thead>
            <tr>
              <th style={{ ...thStyle, width: "30%" }} />
              <th style={{ ...thStyle, color: COLORS.pass }}>Safe Instructions</th>
              <th style={{ ...thStyle, color: COLORS.fail }}>Aggressive Instructions</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => {
              const rowOpacity = spring({
                frame: frame - 15 - i * 12,
                fps,
                config: { damping: 20 },
              });
              const isFailRow = row.aggressive.includes("FAIL") || row.aggressive.includes("ESCALATION") || row.aggressive.includes("BLOCKED");
              return (
                <tr key={i} style={{ opacity: rowOpacity }}>
                  <td style={{ ...tdStyle, color: COLORS.muted, fontWeight: 600 }}>{row.label}</td>
                  <td style={{ ...tdStyle, fontFamily: FONT.mono, fontSize: 20 }}>{row.safe}</td>
                  <td
                    style={{
                      ...tdStyle,
                      fontFamily: FONT.mono,
                      fontSize: 20,
                      color: isFailRow ? COLORS.fail : COLORS.text,
                      fontWeight: isFailRow ? 700 : 400,
                    }}
                  >
                    {row.aggressive}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </AbsoluteFill>
  );
}

const thStyle: CSSProperties = {
  textAlign: "left",
  padding: "16px 20px",
  borderBottom: `2px solid ${COLORS.border}`,
  fontSize: 22,
  fontWeight: 700,
};

const tdStyle: CSSProperties = {
  padding: "14px 20px",
  borderBottom: `1px solid ${COLORS.border}`,
  color: COLORS.text,
};

// Scene 8: Closing (1:45–2:00)
function ClosingScene() {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const tagline1 = spring({ frame, fps, config: { damping: 20 } });
  const tagline2 = spring({ frame: frame - 30, fps, config: { damping: 20 } });
  const cta = spring({ frame: frame - 75, fps, config: { damping: 20 } });
  const links = spring({ frame: frame - 100, fps, config: { damping: 20 } });

  return (
    <AbsoluteFill
      style={{
        background: `linear-gradient(135deg, ${COLORS.bg} 0%, #1a0a2e 50%, ${COLORS.bg} 100%)`,
        justifyContent: "center",
        alignItems: "center",
      }}
    >
      <div style={{ textAlign: "center" }}>
        <div
          style={{
            opacity: tagline1,
            fontSize: 42,
            fontFamily: FONT.sans,
            fontWeight: 700,
            color: COLORS.text,
            lineHeight: 1.6,
          }}
        >
          Same model. Same tools. Different instructions.
        </div>
        <div
          style={{
            opacity: tagline2,
            fontSize: 52,
            fontFamily: FONT.sans,
            fontWeight: 800,
            color: COLORS.accent,
            marginTop: 20,
            marginBottom: 60,
          }}
        >
          CMDR caught it.
        </div>
        <div
          style={{
            opacity: cta,
            fontSize: 30,
            fontFamily: FONT.sans,
            color: COLORS.muted,
            marginBottom: 40,
          }}
        >
          Govern agents before they govern you.
        </div>
        <div
          style={{
            opacity: links,
            fontSize: 22,
            fontFamily: FONT.mono,
            color: COLORS.cyan,
            lineHeight: 2,
          }}
        >
          github.com/lethaltrifecta/replay
          <br />
          github.com/lethaltrifecta/freeze-mcp
        </div>
      </div>
    </AbsoluteFill>
  );
}

// ─── Main Composition ───

export const DemoVideo: React.FC = () => {
  return (
    <AbsoluteFill style={{ background: COLORS.bg }}>
      {/* Scene 1: Title + Problem (0:00–0:15) */}
      <Sequence from={0} durationInFrames={450}>
        <TitleScene />
      </Sequence>

      {/* Scene 2: Terminal Demo (0:15–0:35) */}
      <Sequence from={450} durationInFrames={600}>
        <TerminalScene />
      </Sequence>

      {/* Scene 3: Divergence Engine (0:35–0:50) */}
      <Sequence from={1050} durationInFrames={450}>
        <DivergenceScene />
      </Sequence>

      {/* Scene 4: Shadow Replay (0:50–1:05) */}
      <Sequence from={1500} durationInFrames={450}>
        <ShadowReplayScene />
      </Sequence>

      {/* Scene 5: Gauntlet (1:05–1:15) */}
      <Sequence from={1950} durationInFrames={300}>
        <GauntletScene />
      </Sequence>

      {/* Scene 6: Poisoned Agents (1:15–1:30) */}
      <Sequence from={2250} durationInFrames={450}>
        <PoisonedAgentScene />
      </Sequence>

      {/* Scene 7: Real Model Results (1:30–1:45) */}
      <Sequence from={2700} durationInFrames={450}>
        <VerdictTableScene />
      </Sequence>

      {/* Scene 8: Closing (1:45–2:00) */}
      <Sequence from={3150} durationInFrames={450}>
        <ClosingScene />
      </Sequence>
    </AbsoluteFill>
  );
};
