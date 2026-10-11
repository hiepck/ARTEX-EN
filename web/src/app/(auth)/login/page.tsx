"use client";

import { useEffect, useRef, useState } from "react";

import { useRouter } from "next/navigation";

import { AlertTriangle, ShieldCheck } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { auth } from "@/lib/auth";

export default function LoginPage() {
  const router = useRouter();
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(true);
  const [agreed, setAgreed] = useState(false);
  const [termsOpen, setTermsOpen] = useState(false);
  const [readToEnd, setReadToEnd] = useState(false);
  const termsBodyRef = useRef<HTMLDivElement>(null);

  // "Agree" only becomes clickable after scrolling to the bottom of the terms (including when they fit on screen without scrolling).
  function handleTermsScroll() {
    const el = termsBodyRef.current;
    if (!el) return;
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - 8) setReadToEnd(true);
  }

  useEffect(() => {
    if (!termsOpen) return;
    // Reset it on open, and handle the case where the content is shorter than one screen and cannot be scrolled at all.
    setReadToEnd(false);
    const el = termsBodyRef.current;
    if (el && el.scrollHeight <= el.clientHeight + 8) setReadToEnd(true);
  }, [termsOpen]);

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
    if (!agreed) {
      setError("Please read and accept the Terms of Use first");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const { token } = await api.login("Penred", password);
      auth.setToken(token);
      window.location.replace("/function/tasks");
    } catch {
      setError("The username or password is wrong");
    } finally {
      setLoading(false);
    }
  }

  if (checking) {
    return (
      <div role="status" className="flex min-h-dvh items-center justify-center text-muted-foreground">
        Checking the login state...
      </div>
    );
  }

  return (
    <div className="flex h-dvh">
      {/* Left panel */}
      <div className="hidden flex-col items-center justify-center bg-primary p-12 text-center lg:flex lg:w-1/3">
        <div className="relative flex items-center justify-center">
          <div className="absolute size-80 rounded-full border border-primary-foreground/10" />
          <div className="absolute size-60 rounded-full border border-primary-foreground/15" />
          <div className="absolute size-40 rounded-full border border-primary-foreground/20" />
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/logo.png" alt="Penred" width={160} height={160} className="relative brightness-0 invert" />
        </div>
      </div>

      {/* Right panel */}
      <div className="flex w-full items-center justify-center bg-background p-8 lg:w-2/3">
        <div className="w-full max-w-md space-y-10 py-24 lg:py-32">
          <div className="space-y-4 text-center">
            <h2 className="text-2xl font-medium tracking-tight">Sign in</h2>
            <p className="mx-auto max-w-xl text-muted-foreground">Welcome back; enter your password to continue using Penred</p>
          </div>
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div className="space-y-1.5">
              <Label htmlFor="username">Username</Label>
              <Input id="username" value="Penred" readOnly className="bg-muted text-muted-foreground" />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter the password"
                autoFocus
                autoComplete="current-password"
              />
            </div>
            <div className="flex items-start gap-2">
              <Checkbox
                id="agree-terms"
                checked={agreed}
                onCheckedChange={(v) => setAgreed(v === true)}
                className="mt-0.5"
              />
              <Label htmlFor="agree-terms" className="text-sm font-normal leading-relaxed text-muted-foreground">
                I have read and accept the
                <button
                  type="button"
                  onClick={() => setTermsOpen(true)}
                  className="mx-0.5 font-medium text-primary underline-offset-4 hover:underline"
                >
                  Terms of Use
                </button>
              </Label>
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <Button type="submit" className="w-full" disabled={loading || !password || !agreed}>
              {loading ? "Signing in..." : "Sign in"}
            </Button>
          </form>
        </div>
      </div>

      <Dialog open={termsOpen} onOpenChange={setTermsOpen}>
        <DialogContent className="gap-0 p-0 sm:max-w-2xl">
          <DialogHeader className="flex-row items-center gap-3 border-b px-6 py-4">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <ShieldCheck className="size-5" />
            </div>
            <div className="space-y-0.5">
              <DialogTitle className="text-base">Penred Terms of Use and Disclaimer</DialogTitle>
              <p className="text-xs text-muted-foreground">
                Version v1.0 - effective 2026-09-18 - please read all of the terms below in full before signing in
              </p>
            </div>
          </DialogHeader>

          <div
            ref={termsBodyRef}
            onScroll={handleTermsScroll}
            className="max-h-[60vh] space-y-5 overflow-y-auto px-6 py-5 text-sm leading-relaxed text-muted-foreground"
          >
            <p className="rounded-lg border bg-muted/40 p-3 text-foreground/80">
              These Terms of Use and Disclaimer (the "Terms") form the agreement between you and the Penred
              project's authors and contributors regarding your use of this software. Please read them carefully and make sure you understand every clause before using the software, in particular the disclaimer, limitation of liability and prohibition clauses marked in bold or highlighted.
              <span className="font-medium text-foreground">
                {" "}
                By downloading, installing, accessing or using this software in any way, you are deemed to have read, understood and agreed to be bound by all of these Terms.
              </span>
            </p>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  1
                </span>
                Article 1 - Definitions and the open-source licence
              </h4>
              <p className="pl-7">
                This software (Penred) is an open-source program released under the GNU Affero General Public License
                v3.0 (AGPL-3.0). You may freely use, copy, modify and distribute it under that licence; but any derivative work (including an online service offered to third parties over a network) must likewise be open-sourced under
                AGPL-3.0 with its complete corresponding source code made available to its users. The full text of AGPL-3.0 in the accompanying LICENSE file prevails.
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  2
                </span>
                Article 2 - Permitted scope of use
              </h4>
              <p className="pl-7">
                This software is provided solely for personal study, code research and the discussion of security principles, and for technical validation in a locally isolated environment you set up yourself -- that is, for non-offensive, non-destructive purposes such as learning, academic research and code review. Except as expressly permitted by this article, you may not use this software for any other purpose.
              </p>
            </section>

            <section className="space-y-2">
              <h4 className="flex items-center gap-2 font-medium text-destructive">
                <span className="flex size-5 items-center justify-center rounded-md bg-destructive/10 text-xs font-semibold text-destructive">
                  3
                </span>
                <AlertTriangle className="size-4" />
                Article 3 - Prohibited conduct
              </h4>
              <ul className="ml-7 list-decimal space-y-1.5 rounded-lg border border-destructive/20 bg-destructive/5 p-3 pl-8 text-foreground/80 marker:text-destructive/70">
                <li>
                  Scanning, probing, exploiting or attacking any website, online service, or networked system belonging to another person or a third party is strictly forbidden (whether or not you have authorization, and whether or not it is your own asset);
                </li>
                <li>Using this software for any real penetration test, offensive/defensive exercise, red/blue team exercise or production environment is strictly forbidden;</li>
                <li>Using this software for unlawful intrusion, data theft, extortion, denial of service (DoS/DDoS) or any destructive or criminal activity is strictly forbidden;</li>
                <li>Removing, altering or circumventing any copyright, licence or security notice in this software or its output is strictly forbidden;</li>
                <li>Any conduct that breaches the laws, regulations or rules of your country or region is strictly forbidden.</li>
              </ul>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  4
                </span>
                Article 4 - Intellectual property
              </h4>
              <p className="pl-7">
                The copyright and related intellectual property rights in this software belong to the project's authors and contributors, who grant you the corresponding rights within the scope set out by
                AGPL-3.0. Beyond the rights that licence expressly grants, these Terms grant you no other right, whether express or implied.
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  5
                </span>
                Article 5 - Data and privacy
              </h4>
              <p className="pl-7">
                This software is a self-hosted open-source program; the authors operate no centralized service and neither collect nor upload your usage data. All data you produce, process or come into contact with while using it remains under your control, and its lawfulness and security are your responsibility; any consequence of mishandling it is yours alone to bear.
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  6
                </span>
                Article 6 - Compliance and legal responsibility
              </h4>
              <p className="pl-7">
                You must comply with all of the laws and regulations of your country or region governing cybersecurity, data security and personal information protection, and computer crime (in mainland China these include, among others, the Cybersecurity Law, the Data Security Law, the Personal Information Protection Law and the related judicial interpretations).
                <span className="font-medium text-foreground">
                  {" "}
                  All legal liability and consequences arising from your breach of those laws and regulations or of these Terms rest with you alone and have nothing to do with the software's authors and contributors.
                </span>
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  7
                </span>
                Article 7 - Disclaimer and limitation of liability
              </h4>
              <p className="pl-7">
                This software is provided "AS IS" and "AS
                AVAILABLE", with no warranty of any kind, express or implied, including without limitation any warranty of merchantability, fitness for a particular purpose, accuracy or non-infringement. To the maximum extent permitted by applicable law, the software's authors and contributors are not liable for any direct, indirect, incidental, special or consequential loss arising from the use of, or the inability to use, this software (however it was used), including without limitation data loss, system damage, business interruption, lost profits or legal disputes.
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  8
                </span>
                Article 8 - Changes to the Terms and final interpretation
              </h4>
              <p className="pl-7">
                The authors may update these Terms from time to time as the law or the project requires; an updated version is published with the project and takes effect on the day it is published, and your continued use of the software is deemed acceptance of the revised Terms. To the extent permitted by law, the right of final interpretation of these Terms rests with the project's authors. If any clause is held invalid, the remaining clauses are unaffected.
              </p>
            </section>
          </div>

          <DialogFooter className="mx-0 mb-0 flex-col items-stretch gap-2 rounded-b-xl px-6 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-xs text-muted-foreground">
              {readToEnd ? "You have read all of the terms" : "Scroll the terms to the bottom before confirming"}
            </p>
            <DialogClose asChild>
              <Button
                type="button"
                disabled={!readToEnd}
                onClick={() => {
                  setAgreed(true);
                  setError("");
                }}
              >
                I have read and accept all of the terms
              </Button>
            </DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
