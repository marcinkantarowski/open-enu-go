# A class nobody scans is a style nobody gets

Tailwind 4 generates a utility only for a class it has *seen*, and it looks only in the app it
is building. A class used only inside a shared layer is never generated, and nothing says so.

## The trap
The shared components live in `ui-kit/`. Tailwind scans the app being built - `frontend/`,
`manager/` or `landing/` - and `ui-kit/` is none of them. A class an app's own page uses is
generated; a class used **only** in a shared component is not. In the tenant app,
`bg-brand-50` is generated because the layout uses it; `bg-brand-600` is not, because only
`UiButton` (`ui-kit/app/components/ui/Button.vue`) does.

The result is a primary button with no background, cards with no radius and no shadow, table
headers with no weight. Every template is correct; the stylesheet the browser receives simply
holds no rule for `bg-brand-600` or `rounded-card`. Theme tokens go the same way: an unused
`--radius-card` is dropped from the output, so even the variable is missing.

Nothing fails. There is no build error, no console warning and no 404: an unknown class is a
class with no rule. The page renders, a test clicks the transparent button by its test id,
and the result reads as taste rather than as a bug - "it looks so-so" - so the instinct is to
restyle instead of asking why the existing styles are not applied.

## The rule
A layer that ships components names itself as a Tailwind source. When something looks
unstyled, read the computed style before writing CSS: a class in the DOM with no rule behind
it is this problem, not a design problem.

## Where it is held
- `ui-kit/app/assets/css/main.css` declares `@source "../../";`, relative to the file, so it
  holds wherever the layer is installed. Do not remove the line; add one for any new package
  that carries templates.
- `e2e/tests/login.spec.ts` ("the shared components arrive with their styles") asserts that
  the sign-in button's computed background is neither `transparent` nor `rgba(0, 0, 0, 0)`.
  Its colour is a class only the shared layer uses, on a screen every project renders.
