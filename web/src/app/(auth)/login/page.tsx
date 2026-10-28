"use client";

import { useEffect, useState } from "react";

import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { auth } from "@/lib/auth";

import { AuthCard } from "../_components/auth-card";

export default function LoginPage() {
  const router = useRouter();
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(true);

  useEffect(() => {
    // Already logged in: go straight into the main UI (under a static export there is no middleware to do this redirect).
    const token = auth.getToken();
    if (token) {
      // localStorage may still hold the credentials while the cookie is gone. Sync first, then make a fresh request,
      // so a server-side guard or the router cache cannot send the redirect back to a login page still in the checking state.
      auth.setToken(token);
      window.location.replace("/function/tasks");
      return;
    }
    api
      .authStatus()
      .then(({ initialized }) => {
        if (!initialized) router.replace("/setup");
      })
      .catch(() => setError("The backend service could not be reached"))
      .finally(() => setChecking(false));
  }, [router]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      const { token } = await api.login("admin", password);
      auth.setToken(token);
      window.location.replace("/function/tasks");
    } catch {
      setError("The password is wrong");
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
    <AuthCard title="Sign in" subtitle="Enter your password to continue.">
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="password" className="text-slate-700">
            Password
          </Label>
          <Input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Enter your password"
            autoFocus
            autoComplete="current-password"
            className="border-slate-300 bg-white"
          />
        </div>
        {error && <p className="text-red-600 text-sm">{error}</p>}
        <Button
          type="submit"
          className="w-full bg-slate-900 text-white hover:bg-slate-800"
          disabled={loading || !password}
        >
          {loading ? "Signing in..." : "Sign in"}
        </Button>
      </form>
    </AuthCard>
  );
}
