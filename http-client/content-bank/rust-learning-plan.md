# Rust Learning Plan: Go Dev + FP Background

## Your Advantages Going In
- Go: concurrency mental model, interfaces, error-as-value
- FP: immutability, higher-order functions, algebraic types, composition
- You'll find Rust's `Option`/`Result`, iterators, and closures very natural

---

## Phase 1 — Ownership & Borrowing (2–3 weeks)
*This is the only truly foreign concept. Everything else maps to something you know.*

- Read **The Book** ch. 1–10 (doc.rust-lang.org/book) — don't skip, it's excellent
- Mental model: think of the borrow checker as a compile-time race detector (you know why races are bad from Go)
- Key concepts: move semantics, `&T` vs `&mut T`, lifetimes (just the basics for now)
- Practice: rewrite small Go utilities in Rust — file readers, string processors

**Milestone:** write a CLI tool that reads a file and processes lines without fighting the compiler.

---

## Phase 2 — Type System & FP Mapping (2–3 weeks)
*Your FP background makes this phase fast.*

| FP / Haskell concept | Rust equivalent |
|---|---|
| `Maybe` | `Option<T>` |
| `Either` | `Result<T, E>` |
| Typeclasses | Traits |
| ADTs | `enum` with data |
| Functor/map | `.map()` on iterators, Option, Result |
| Composition | chained iterators, `and_then` |

- Deep dive: traits, generics, `impl Trait` vs `dyn Trait`
- Iterator chains — Rust's lazy iterators will feel like home
- `From`/`Into`, `Display`, `Debug` — the common traits

**Milestone:** build a data transformation pipeline using iterators with zero intermediate `Vec`s.

---

## Phase 3 — Error Handling & Structs (1–2 weeks)
*Go's explicit error returns prepared you well.*

- `?` operator (like Go's `if err != nil` but composable)
- `thiserror` crate for defining errors
- `anyhow` crate for application-level error handling
- Struct methods, associated functions, `impl` blocks
- Builder pattern (very common in Rust APIs)

**Milestone:** a small HTTP client that fetches JSON, deserializes with `serde`, and handles errors cleanly.

---

## Phase 4 — Concurrency (2–3 weeks)
*Different from Go but your mental model helps.*

| Go | Rust |
|---|---|
| goroutines | `tokio::spawn` / `async fn` |
| channels | `tokio::sync::mpsc`, `std::sync::mpsc` |
| `sync.Mutex` | `Mutex<T>` (data-owning!) |
| `sync.WaitGroup` | `JoinHandle`, `join!` macro |
| `context.Context` | `CancellationToken` (tokio) |

- Learn `async`/`await` with **Tokio**
- Understand `Send` + `Sync` traits — the compiler enforces what Go's race detector catches at runtime
- Note: Rust's `Mutex<T>` wraps the *data*, not just a lock — very different from Go

**Milestone:** a concurrent web scraper or async task queue using Tokio.

---

## Phase 5 — Ecosystem & Idioms (ongoing)
*Getting productive, not just correct.*

- **`serde`** — JSON/serialization (you'll use this constantly)
- **`axum`** or **`actix-web`** — if you build web services
- **`clap`** — CLI argument parsing
- **`tracing`** — structured logging (like Go's `slog`)
- **`cargo`** — already better than Go modules in most ways
- Learn Rust's workspace model for multi-crate projects

---

## Key Mindset Shifts from Go

1. **No GC** — you must think about where data lives, but the compiler guides you
2. **No nil** — `Option<T>` forces you to handle absence explicitly (your FP brain loves this)
3. **Enums are powerful** — Rust enums are full ADTs, not just integer labels like Go's `iota`
4. **Traits ≠ Interfaces** — traits are more powerful (blanket impls, associated types) but more explicit
5. **Macros** — `println!`, `vec!`, `serde` derive — don't fear them, but don't write them early
6. **Compilation is slow** — use `cargo check` constantly, save `cargo build` for milestones

---

## Recommended Resources

- 📖 [The Rust Book](https://doc.rust-lang.org/book/) — start here
- 📖 [Rust by Example](https://doc.rust-lang.org/rust-by-example/) — great for FP folks
- 📖 [Rustlings](https://github.com/rust-lang/rustlings) — hands-on exercises
- 📖 [Zero to Production in Rust](https://www.zero2prod.com/) — if you build web services
- 📖 [Tokio Tutorial](https://tokio.rs/tokio/tutorial) — for async/concurrency

---

## Total Timeline
| Phase | Duration |
|---|---|
| Ownership & Borrowing | 2–3 weeks |
| Type System & FP | 2–3 weeks |
| Error Handling | 1–2 weeks |
| Concurrency | 2–3 weeks |
| Ecosystem | ongoing |
| **Productive in Rust** | **~2–3 months** |
