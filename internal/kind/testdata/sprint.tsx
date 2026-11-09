import { useMemo, useState } from "react";
import { Area, AreaChart, Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip as ChartTip, XAxis, YAxis } from "recharts";
import { Activity, CheckCircle2, Flame, RefreshCw, Users } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Progress } from "@/components/ui/progress";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

type Day = { day: string; done: number; added: number };
const seed: Day[] = Array.from({ length: 10 }, (_, i) => ({ day: `D${i + 1}`, done: 2 + ((i * 7) % 6), added: (i * 3) % 4 }));

export default function SprintHealth() {
  const [days, setDays] = useState<Day[]>(seed);
  const total = useMemo(() => days.reduce((n, d) => n + d.done, 0), [days]);
  const burn = useMemo(() => {
    let left = 60;
    return days.map((d) => ({ day: d.day, left: (left -= d.done - d.added) }));
  }, [days]);
  return (
    <TooltipProvider>
      <main className="mx-auto max-w-4xl space-y-6 p-6">
        <header className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="flex items-center gap-2 text-2xl font-semibold"><Activity className="size-6 text-primary" /> Sprint 42 health</h1>
            <p className="text-sm text-muted-foreground">Written by an agent as one TSX file.</p>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant="secondary"><Users className="mr-1 size-3" />6 people</Badge>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="outline" size="sm" onClick={() => setDays(days.map((d) => ({ ...d, done: (d.done + 3) % 8 })))}>
                  <RefreshCw className="size-4" />Shuffle
                </Button>
              </TooltipTrigger>
              <TooltipContent>Re-roll the sample data</TooltipContent>
            </Tooltip>
          </div>
        </header>
        <div className="grid gap-4 sm:grid-cols-3">
          <Card><CardHeader><CardDescription>Done</CardDescription><CardTitle className="text-3xl">{total}</CardTitle></CardHeader>
            <CardContent><Progress value={Math.min(100, (total / 60) * 100)} /></CardContent></Card>
          <Card><CardHeader><CardDescription>Streak</CardDescription><CardTitle className="flex items-center gap-2 text-3xl"><Flame className="size-6 text-destructive" />7d</CardTitle></CardHeader></Card>
          <Card><CardHeader><CardDescription>Gate</CardDescription><CardTitle className="flex items-center gap-2 text-xl"><CheckCircle2 className="size-5 text-primary" />make verify</CardTitle></CardHeader></Card>
        </div>
        <Tabs defaultValue="throughput">
          <TabsList><TabsTrigger value="throughput">Throughput</TabsTrigger><TabsTrigger value="burn">Burn-down</TabsTrigger></TabsList>
          <TabsContent value="throughput">
            <Card><CardContent className="h-64 pt-5">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={days}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--line)" />
                  <XAxis dataKey="day" stroke="var(--muted)" /><YAxis stroke="var(--muted)" />
                  <ChartTip contentStyle={{ background: "var(--plate)", border: "1px solid var(--line)" }} />
                  <Bar dataKey="done" fill="var(--chart-1)" radius={4} /><Bar dataKey="added" fill="var(--chart-2)" radius={4} />
                </BarChart>
              </ResponsiveContainer>
            </CardContent></Card>
          </TabsContent>
          <TabsContent value="burn">
            <Card><CardContent className="h-64 pt-5">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={burn}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--line)" />
                  <XAxis dataKey="day" stroke="var(--muted)" /><YAxis stroke="var(--muted)" />
                  <ChartTip contentStyle={{ background: "var(--plate)", border: "1px solid var(--line)" }} />
                  <Area dataKey="left" stroke="var(--chart-1)" fill="var(--chart-1)" fillOpacity={0.25} />
                </AreaChart>
              </ResponsiveContainer>
            </CardContent></Card>
          </TabsContent>
        </Tabs>
        <Dialog>
          <DialogTrigger asChild><Button>Details</Button></DialogTrigger>
          <DialogContent><DialogTitle>How this page was made</DialogTitle>
            <DialogDescription>One TSX file, compiled by pictor at publish time. No build step for the author.</DialogDescription></DialogContent>
        </Dialog>
      </main>
    </TooltipProvider>
  );
}
