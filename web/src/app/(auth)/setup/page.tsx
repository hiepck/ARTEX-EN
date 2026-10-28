"use client";

import { useEffect, useState } from "react";

import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { auth } from "@/lib/auth";

import { AuthCard } from "../_components/auth-card";

export default function SetupPage() {
  const router = useRouter();
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(true);
  // When the initialization status cannot be fetched we must not default to
  // treating the instance as "uninitialized" — that would show the setup form on
  // an instance that already has a password, and a user filling it in would
  // overwrite it. Close the entry instead.
  const [unavailable, setUnavailable] = useState(false);

  useEffect(() => {
    api
      .authStatus()
      .then(({ initialized }) => {
        if (initialized) router.replace("/login");
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : "The backend service could not be reached");
        setUnavailable(true);
      })
      .finally(() => setChecking(false));
  }, [router]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (password !== confirm) {
      setError("The two passwords do not match");
      return;
    }
    if (password.length < 8) {
      setError("The password must be at least 8 characters long");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const { token } = await api.initPassword(password);
      auth.setToken(token);
      router.replace("/function/tasks");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Initialization failed");
    } finally {
      setLoading(false);
    }
  }

  if (checking) {
    return (
      <div role="status" className="flex min-h-dvh items-center justify-center bg-slate-100 text-slate-500 text-sm">
        Loading...
      </div>
    );
  }

  return (
    <AuthCard
      title={unavailable ? "Service unavailable" : "Set a password"}
      subtitle={
        unavailable
          ? "The backend is temporarily unreachable, so setup is disabled to avoid overwriting an existing password."
          : "This is the first run of this console. Choose a password (at least 8 characters)."
      }
    >
      {unavailable ? (
        <div className="space-y-4">
          {error && <p className="text-center text-red-600 text-sm">{error}</p>}
          <Button type="button" className="w-full bg-slate-900 text-white hover:bg-slate-800" onClick={() => window.location.reload()}>
            Retry
          </Button>
        </div>
      ) : (
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="password" className="text-slate-700">
              New password
            </Label>
            <Input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="At least 8 characters"
              autoFocus
              autoComplete="new-password"
              className="border-slate-300 bg-white"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="confirm" className="text-slate-700">
              Confirm password
            </Label>
            <Input
              id="confirm"
              type="password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              placeholder="Type the password again"
              autoComplete="new-password"
              className="border-slate-300 bg-white"
            />
          </div>
          {error && <p className="text-red-600 text-sm">{error}</p>}
          <Button
            type="submit"
            className="w-full bg-slate-900 text-white hover:bg-slate-800"
            disabled={loading || !password || !confirm}
          >
            {loading ? "Saving..." : "Set password and sign in"}
          </Button>
        </form>
      )}
    </AuthCard>
  );
}
