# SVG Icons

This directory contains all SVG icons used in the application.

## Icon Standards

All SVG icons in this directory must follow these standards:

### Root `<svg>` Attributes

```xml
<svg
  width="20"
  height="20"
  viewBox="0 0 20 20"
  stroke="currentColor"
  stroke-width="1.5"
  fill="none"
>
```

### Nested Elements

- **DO NOT** include `stroke`, `stroke-width`, or `fill` attributes on nested elements
- **DO NOT** use `<rect>` or `<circle>` elements, only `<path>` elements
- These attributes should only be on the root `<svg>` tag
- This allows the icon to inherit the color from the parent component

### Nav Icons
- for nav_[icon].svg, use class `filled` for a `<path>` element to get a filled icon for a selected state
- for nav_[icon].svg, use class `filled` for a `<path>` element to get a "negative" part of it

### Why These Standards?

1. **Consistent sizing**: All icons are 20x20px for uniform appearance
2. **Color inheritance**: `stroke="currentColor"` allows icons to inherit text color
3. **Stroke width**: `stroke-width="1.5"` provides consistent line thickness
4. **Clean nesting**: No attributes on nested elements prevents styling conflicts

## Validation Script

Use the validation script to check and fix SVG icons:

### Check for Issues

```bash
node validate-icons.mjs
```

This will:
- Check all `.svg` files in the icons directory
- Report any files that don't meet the standards
- Exit with error code 1 if issues are found

### Auto-Fix Issues

```bash
node validate-icons.mjs --fix
```

This will:
- Automatically fix all non-compliant SVG files
- Update root `<svg>` attributes to match standards
- Remove `stroke`, `stroke-width`, and `fill` from nested elements
- Report which files were fixed

### Example Output

**Validation mode:**
```
🔍 Validating SVG icons...

❌ arrow_down.svg
  - stroke-width="2.5" (expected "1.5")

📊 Summary:
   Total files checked: 31
   Files with issues: 1
   Total issues found: 1

💡 Run with --fix to automatically fix these issues:
   node validate-icons.mjs --fix
```

**Fix mode:**
```
🔧 Fixing SVG icons...

✅ arrow_down.svg
  - stroke-width="2.5" (expected "1.5")

📊 Summary:
   Total files checked: 31
   Files with issues: 1
   Total issues found: 1
   Files fixed: 1
```

## Adding New Icons

When adding new SVG icons:

1. Add the `.svg` file to this directory
2. Run the validation script: `node validate-icons.mjs`
3. If issues are found, run: `node validate-icons.mjs --fix`
4. Verify the icon displays correctly in the UI

## Script Details

The `validate-icons.mjs` script:
- Written in ESM (ES Modules) for Node.js
- Uses regex parsing for SVG manipulation
- Preserves all other attributes and elements
- Only modifies the specific attributes that need fixing