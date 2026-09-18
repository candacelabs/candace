import { useEffect, useMemo, useRef, useState, type CSSProperties, type RefObject } from "react";
import { Badge, Box, Button, Card, Container, Group, Paper, SimpleGrid, Stack, Text, TextInput, Title } from "@mantine/core";
import type { Health, Model, Session, TelemetrySnapshot, TranscriptItem, Worktree } from "./api/client";
import { absoluteTime, relativeTime, worktreeLabel } from "./format";
import { csfDestinations } from "./navigation";

type ActivityItem = TranscriptItem & { sessionName: string };

type HomePageProps = {
  kanban?: boolean;
  sessions: Session[];
  worktrees: Worktree[];
  models: Model[];
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  modelsLoading: boolean;
  modelsError: string | null;
  observedAt: Date | null;
  menuButtonRef: RefObject<HTMLButtonElement>;
  onMenu: () => void;
  onNewSession: () => void;
  onRefresh: () => void;
  health?: Health | null;
  telemetry?: TelemetrySnapshot | null;
  pendingBySession?: Record<string, number>;
  activity?: ActivityItem[];
};

const statuses = ["running", "idle", "failed"] as const;

function statusColor(status: string): string {
  if (status === "running") return "teal";
  if (status === "failed") return "red";
  return "gray";
}

function StatusChip({ label, value, color = "gray" }: { label: string; value: string | number; color?: string }) {
  return <Paper className="flight-chip" withBorder radius="sm" p="xs">
    <Text size="xs" c="dimmed" tt="uppercase" fw={700}>{label}</Text>
    <Text size="lg" fw={800} c={color}>{value}</Text>
  </Paper>;
}

function ApprovalChip({ count, sessionId }: { count: number; sessionId: string | undefined }) {
  const content = <><Text size="xs" c="dimmed" tt="uppercase" fw={700}>Approvals</Text><Text size="lg" fw={800} c={count > 0 ? "orange" : "gray"}>{count}</Text></>;
  return sessionId === undefined ? <Paper className="flight-chip" withBorder radius="sm" p="xs">{content}</Paper> : <Paper component="a" href={`#/sessions/${sessionId}`} className="flight-chip approval-chip" withBorder radius="sm" p="xs" aria-label={`Open pending approval session`} title="Open the next session awaiting approval">{content}</Paper>;
}

function ServiceDot({ label, healthy }: { label: string; healthy: boolean | null }) {
  const color = healthy === null ? "yellow" : healthy ? "teal" : "red";
  return <Badge color={color} variant="light" size="lg" radius="sm"><span className="service-dot" />{label} · {healthy === null ? "unknown" : healthy ? "online" : "degraded"}</Badge>;
}

function UsageStrip({ telemetry }: { telemetry: TelemetrySnapshot | null | undefined }) {
  const sessions = telemetry?.sessions ?? [];
  const premium = sessions.reduce((total, session) => total + (session.premiumRequests ?? 0), 0);
  const topModel = sessions.slice().sort((left, right) => (right.premiumRequests ?? 0) - (left.premiumRequests ?? 0))[0]?.model ?? "—";
  const extended = telemetry as (TelemetrySnapshot & { nanoAiu?: number; quota?: { used: number; entitlement: number; resetAt?: string } }) | null | undefined;
  const quota = extended?.quota;
  const used = quota?.used ?? premium;
  const entitlement = quota?.entitlement ?? 0;
  const remaining = entitlement > 0 ? Math.max(0, Math.round(((entitlement - used) / entitlement) * 100)) : null;
  const ringStyle = remaining !== null ? { "--usage-angle": `${remaining * 3.6}deg` } as CSSProperties : undefined;
  return <Paper component="section" aria-labelledby="usage-title" className="usage-strip" withBorder radius="md" p="md">
    <Group justify="space-between" align="flex-start">
      <div><Text id="usage-title" className="eyebrow">AIC / usage</Text><Title order={3} size="h4">Provider budget</Title></div>
      <div className={`usage-ring ${remaining !== null && remaining < 20 ? "critical" : ""}`} style={ringStyle} aria-label={remaining === null ? "Quota unavailable" : `${remaining}% quota remaining`}><strong>{remaining === null ? "—" : `${remaining}%`}</strong><span>left</span></div>
    </Group>
    <SimpleGrid cols={{ base: 2, sm: 4 }} mt="md">
      <div><Text size="xs" c="dimmed">Premium requests</Text><Text fw={800}>{premium.toLocaleString()}</Text></div>
      <div><Text size="xs" c="dimmed">AIU today</Text><Text fw={800}>{extended?.nanoAiu === undefined ? "—" : extended.nanoAiu.toLocaleString()}</Text></div>
      <div><Text size="xs" c="dimmed">Top model</Text><Text fw={800} truncate title={topModel}>{topModel}</Text></div>
      <div><Text size="xs" c="dimmed">Reset</Text><Text fw={800}>{quota?.resetAt === undefined ? "Not reported" : relativeTime(quota.resetAt, new Date())}</Text></div>
    </SimpleGrid>
  </Paper>;
}

function ActivityStream({ activity }: { activity: ActivityItem[] }) {
  return <Paper component="section" aria-labelledby="activity-title" className="activity-stream" withBorder radius="md" p="md">
    <Group justify="space-between"><Title id="activity-title" order={3} size="h4">Recent activity</Title><Text size="xs" c="dimmed">last {activity.length}</Text></Group>
    <Stack gap={0} mt="sm">
      {activity.length === 0 && <Text size="sm" c="dimmed">No retained turns yet.</Text>}
      {activity.map((item) => <div className="activity-row" key={`${item.sessionId}-${item.seq}`}>
        <Text size="xs" c="dimmed" component="time" dateTime={item.occurredAt} title={absoluteTime(item.occurredAt)}>{relativeTime(item.occurredAt, new Date())}</Text>
        <Text size="xs" fw={700} truncate title={item.sessionName}>{item.sessionName}</Text>
        <Text size="xs" className={item.kind === "toolCall" || item.kind === "toolResult" ? "mono" : ""} truncate title={item.text}>{item.kind === "toolCall" ? `◆ ${item.toolName ?? "tool"} ${item.text}` : item.kind === "toolResult" ? `↳ ${item.toolName ?? "result"} ${item.text}` : item.text || item.kind}</Text>
      </div>)}
    </Stack>
  </Paper>;
}

function SessionDeck({ sessions, worktrees, pendingBySession, query, onQuery, loading }: { sessions: Session[]; worktrees: Worktree[]; pendingBySession: Record<string, number>; query: string; onQuery: (value: string) => void; loading: boolean }) {
  const [status, setStatus] = useState("");
  const cards = useRef<Array<HTMLAnchorElement | null>>([]);
  const filtered = useMemo(() => sessions.filter((session) => (status === "" || session.status === status) && `${session.displayName} ${session.model} ${session.id} ${worktrees.find((tree) => tree.id === session.worktreeId)?.branch ?? ""}`.toLowerCase().includes(query.toLowerCase())), [query, sessions, status, worktrees]);
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "/" && document.activeElement?.tagName !== "INPUT") { event.preventDefault(); document.querySelector<HTMLInputElement>("[data-home-search]")?.focus(); }
      if (!["j", "k"].includes(event.key) || document.activeElement?.tagName === "INPUT") return;
      const current = cards.current.findIndex((card) => card === document.activeElement);
      const next = event.key === "j" ? current + 1 : current - 1;
      cards.current[next < 0 ? 0 : Math.min(next, cards.current.length - 1)]?.focus();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);
  return <Stack component="section" aria-labelledby="sessions-title" gap="sm">
    <Group justify="space-between" align="end"><div><Text className="eyebrow">Live fleet</Text><Title id="sessions-title" order={2} size="h3">Sessions</Title></div><Group><TextInput data-home-search aria-label="Find a task" placeholder="Search sessions /" value={query} onChange={(event) => onQuery(event.currentTarget.value)} /><select aria-label="Filter tasks by status" value={status} onChange={(event) => setStatus(event.currentTarget.value)}><option value="">All statuses</option>{statuses.map((value) => <option key={value} value={value}>{value}</option>)}</select></Group></Group>
    {loading && <SimpleGrid cols={{ base: 1, sm: 2, xl: 3 }} aria-label="Loading tasks"><div className="flight-skeleton" /><div className="flight-skeleton" /><div className="flight-skeleton" /></SimpleGrid>}
    <SimpleGrid cols={{ base: 1, sm: 2, xl: 3 }} spacing="sm">
      {filtered.map((session, index) => {
        const worktree = worktrees.find((tree) => tree.id === session.worktreeId);
        const pending = pendingBySession[session.id] ?? 0;
        return <Card component="a" href={`#/sessions/${session.id}`} ref={(element) => { cards.current[index] = element; }} key={session.id} className="session-deck-card" withBorder radius="md" p="md" aria-label={`Continue ${session.displayName}`}>
          <Group justify="space-between" wrap="nowrap"><Badge color={statusColor(session.status)} variant="light" className={session.status === "running" ? "status-pulse" : ""}>{session.status}</Badge><Text size="xs" c="dimmed" component="time" dateTime={session.updatedAt} title={absoluteTime(session.updatedAt)}>{relativeTime(session.updatedAt, new Date())}</Text></Group>
          <Text fw={800} mt="sm" lineClamp={1}>{session.displayName}</Text>
          <Text size="xs" c="dimmed" mt={4}>{session.model} · {worktree ? worktreeLabel(worktree) : "worktree unavailable"}</Text>
          <Group mt="md" justify="space-between"><Text size="xs" className="mono" c="dimmed">{worktree?.branch ?? "detached"}</Text>{pending > 0 && <Badge color="orange" variant="filled">{pending} approval{pending === 1 ? "" : "s"}</Badge>}</Group>
          {session.status === "failed" && <Text size="xs" c="red" mt="sm">Session failed; open for details.</Text>}
        </Card>;
      })}
    </SimpleGrid>
    {filtered.length === 0 && <Text c="dimmed" size="sm">No sessions match the current search.</Text>}
  </Stack>;
}

export function HomePage(props: HomePageProps) {
  const [query, setQuery] = useState("");
  const pending = props.pendingBySession ?? {};
  const counts = Object.fromEntries(statuses.map((status) => [status, props.sessions.filter((session) => session.status === status).length]));
  const pendingTotal = Object.values(pending).reduce((total, value) => total + value, 0);
  const traces = props.telemetry?.traces ?? [];
  const langfuseHealthy = traces.length === 0 ? null : (traces.find((trace) => trace.state === "failed")?.count ?? 0) === 0;
  return <Box className="home-flight-deck" w="100%" h="100%" style={{ overflowY: "auto" }}>
    <Container size="xl" py="lg" px={{ base: "md", sm: "xl" }}>
      <Stack gap="lg">
        <Group justify="space-between" align="flex-start">
          <Group gap="xs"><Button ref={props.menuButtonRef} hiddenFrom="sm" variant="subtle" aria-label="Open task sidebar" onClick={props.onMenu}>☰</Button><div><Text className="eyebrow">Workbench / home</Text><Title order={1}>Operator flight deck</Title><Text size="sm" c="dimmed">Live session posture, approvals, and provider capacity.</Text></div></Group>
          <Group><Button variant="default" size="xs" loading={props.refreshing} onClick={props.onRefresh}>Refresh overview</Button><Button size="sm" onClick={props.onNewSession}>＋ New task</Button></Group>
        </Group>
        <Group gap="xs" className="flight-chip-row">
          <StatusChip label="Tasks" value={props.loading ? "…" : props.error ? "Unavailable" : props.sessions.length} /><StatusChip label="Working now" value={counts.running} color="teal" /><StatusChip label="Idle" value={counts.idle} /><StatusChip label="Failed" value={counts.failed} color="red" /><ApprovalChip count={pendingTotal} sessionId={Object.entries(pending).find(([, count]) => count > 0)?.[0]} />
          <ServiceDot label="Langfuse" healthy={langfuseHealthy} /><ServiceDot label="CSF" healthy={props.health?.status === undefined ? null : props.health.status === "ok"} />
        </Group>
        <UsageStrip telemetry={props.telemetry} />
        <SessionDeck sessions={props.sessions} worktrees={props.worktrees} pendingBySession={pending} query={query} onQuery={setQuery} loading={props.loading} />
        <ActivityStream activity={props.activity ?? []} />
        {props.error && <><Badge color="red" variant="light">Refresh failed · previous snapshot</Badge><Text c="red" size="xs">{props.error}</Text></>}
        {csfDestinations().length > 0 && <Stack component="section" aria-labelledby="home-explore" gap={4}><Title id="home-explore" order={3} size="h4">Around the workshop</Title><Group gap="xs">{csfDestinations().map((destination) => <Button key={destination.name} component="a" href={destination.href} variant="subtle" size="compact-xs">{destination.name}</Button>)}</Group></Stack>}
      </Stack>
    </Container>
  </Box>;
}
