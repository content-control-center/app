#!/usr/bin/env node

import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

const EXPECTED_WIDTH = '20'
const EXPECTED_HEIGHT = '20'
const EXPECTED_VIEWBOX = '0 0 20 20'
const EXPECTED_STROKE = 'currentColor'
const EXPECTED_STROKE_WIDTH = '1.5'
const EXPECTED_FILL = 'none'

// Parse command line arguments
const args = process.argv.slice(2)
const FIX_MODE = args.includes('--fix')

/**
 * Parse SVG file content and return root element attributes and children
 */
function parseSVG(content) {
  // Extract root <svg> tag
  const svgMatch = content.match(/<svg([^>]*)>([\s\S]*)<\/svg>/i)
  if (!svgMatch) {
    throw new Error('Invalid SVG: No <svg> tag found')
  }

  const rootAttributes = svgMatch[1]
  const innerContent = svgMatch[2]

  return { rootAttributes, innerContent, fullMatch: svgMatch[0] }
}

/**
 * Extract attribute value from a string of attributes
 */
function getAttributeValue(attributes, name) {
  const regex = new RegExp(`${name}="([^"]*)"`, 'i')
  const match = attributes.match(regex)
  return match ? match[1] : null
}

/**
 * Check if attribute exists
 */
function hasAttribute(attributes, name) {
  const regex = new RegExp(`${name}=`, 'i')
  return regex.test(attributes)
}

/**
 * Set or update attribute value
 */
function setAttribute(attributes, name, value) {
  const regex = new RegExp(`${name}="[^"]*"`, 'i')
  if (regex.test(attributes)) {
    // Update existing attribute
    return attributes.replace(regex, `${name}="${value}"`)
  } else {
    // Add new attribute
    return `${attributes} ${name}="${value}"`
  }
}

/**
 * Remove attribute
 */
function removeAttribute(attributes, name) {
  const regex = new RegExp(`\\s*${name}="[^"]*"`, 'gi')
  return attributes.replace(regex, '')
}

/**
 * Remove stroke, stroke-width, and fill from nested elements
 */
function cleanNestedElements(innerContent) {
  // Match all element tags (not closing tags)
  return innerContent.replace(
    /<([a-z][a-z0-9]*)\s+([^>]*?)(\/?)>/gi,
    (match, tagName, attributes, selfClosing) => {
      let cleanedAttrs = attributes

      // Remove stroke, stroke-width, and fill attributes
      cleanedAttrs = removeAttribute(cleanedAttrs, 'stroke')
      cleanedAttrs = removeAttribute(cleanedAttrs, 'stroke-width')
      cleanedAttrs = removeAttribute(cleanedAttrs, 'fill')

      // Clean up extra whitespace
      cleanedAttrs = cleanedAttrs.replace(/\s+/g, ' ').trim()

      return `<${tagName}${cleanedAttrs ? ' ' + cleanedAttrs : ''}${selfClosing}>`
    }
  )
}

/**
 * Check if nested elements have stroke, stroke-width, or fill attributes
 */
function checkNestedElements(innerContent) {
  const issues = []

  // Match all element tags
  const elementRegex = /<([a-z][a-z0-9]*)\s+([^>]*?)(\/?)>/gi
  let match

  while ((match = elementRegex.exec(innerContent)) !== null) {
    const tagName = match[1]
    const attributes = match[2]

    if (hasAttribute(attributes, 'stroke')) {
      issues.push(`  - <${tagName}> has stroke attribute`)
    }
    if (hasAttribute(attributes, 'stroke-width')) {
      issues.push(`  - <${tagName}> has stroke-width attribute`)
    }
    if (hasAttribute(attributes, 'fill')) {
      issues.push(`  - <${tagName}> has fill attribute`)
    }
  }

  return issues
}

/**
 * Validate and optionally fix a single SVG file
 */
function validateSVGFile(filePath) {
  const fileName = path.basename(filePath)
  const content = fs.readFileSync(filePath, 'utf-8')

  let issues = []
  let fixed = false

  try {
    let { rootAttributes, innerContent, fullMatch } = parseSVG(content)

    // Check root <svg> dimensions
    const width = getAttributeValue(rootAttributes, 'width')
    const height = getAttributeValue(rootAttributes, 'height')
    const viewBox = getAttributeValue(rootAttributes, 'viewBox')

    if (width !== EXPECTED_WIDTH) {
      issues.push(`  - width="${width}" (expected "${EXPECTED_WIDTH}")`)
      if (FIX_MODE) {
        rootAttributes = setAttribute(rootAttributes, 'width', EXPECTED_WIDTH)
        fixed = true
      }
    }

    if (height !== EXPECTED_HEIGHT) {
      issues.push(`  - height="${height}" (expected "${EXPECTED_HEIGHT}")`)
      if (FIX_MODE) {
        rootAttributes = setAttribute(rootAttributes, 'height', EXPECTED_HEIGHT)
        fixed = true
      }
    }

    if (viewBox !== EXPECTED_VIEWBOX) {
      issues.push(`  - viewBox="${viewBox}" (expected "${EXPECTED_VIEWBOX}")`)
      if (FIX_MODE) {
        rootAttributes = setAttribute(rootAttributes, 'viewBox', EXPECTED_VIEWBOX)
        fixed = true
      }
    }

    // Check root <svg> stroke attributes
    const stroke = getAttributeValue(rootAttributes, 'stroke')
    const strokeWidth = getAttributeValue(rootAttributes, 'stroke-width')
    const fill = getAttributeValue(rootAttributes, 'fill')

    if (stroke !== EXPECTED_STROKE) {
      issues.push(`  - stroke="${stroke}" (expected "${EXPECTED_STROKE}")`)
      if (FIX_MODE) {
        rootAttributes = setAttribute(rootAttributes, 'stroke', EXPECTED_STROKE)
        fixed = true
      }
    }

    if (strokeWidth !== EXPECTED_STROKE_WIDTH) {
      issues.push(`  - stroke-width="${strokeWidth}" (expected "${EXPECTED_STROKE_WIDTH}")`)
      if (FIX_MODE) {
        rootAttributes = setAttribute(rootAttributes, 'stroke-width', EXPECTED_STROKE_WIDTH)
        fixed = true
      }
    }

    if (fill !== EXPECTED_FILL) {
      issues.push(`  - fill="${fill}" (expected "${EXPECTED_FILL}")`)
      if (FIX_MODE) {
        rootAttributes = setAttribute(rootAttributes, 'fill', EXPECTED_FILL)
        fixed = true
      }
    }

    // Check nested elements
    const nestedIssues = checkNestedElements(innerContent)
    if (nestedIssues.length > 0) {
      issues.push(...nestedIssues)
      if (FIX_MODE) {
        innerContent = cleanNestedElements(innerContent)
        fixed = true
      }
    }

    // Write fixed content back to file
    if (FIX_MODE && fixed) {
      const newSVG = `<svg${rootAttributes}>${innerContent}</svg>`
      const newContent = content.replace(fullMatch, newSVG)
      fs.writeFileSync(filePath, newContent, 'utf-8')
    }

    return { fileName, issues, fixed }
  } catch (error) {
    return { fileName, issues: [`  - Error: ${error.message}`], fixed: false }
  }
}

/**
 * Main function
 */
function main() {
  const iconsDir = __dirname
  const files = fs.readdirSync(iconsDir).filter((file) => file.endsWith('.svg'))

  if (files.length === 0) {
    console.log('No SVG files found in icons directory')
    return
  }

  console.log(FIX_MODE ? '🔧 Fixing SVG icons...\n' : '🔍 Validating SVG icons...\n')

  let totalIssues = 0
  let totalFixed = 0
  const results = []

  for (const file of files) {
    const filePath = path.join(iconsDir, file)
    const result = validateSVGFile(filePath)

    if (result.issues.length > 0) {
      results.push(result)
      totalIssues += result.issues.length
      if (result.fixed) {
        totalFixed++
      }
    }
  }

  // Print results
  if (results.length === 0) {
    console.log('✅ All SVG icons are valid!')
  } else {
    results.forEach(({ fileName, issues, fixed }) => {
      console.log(`${fixed ? '✅' : '❌'} ${fileName}`)
      issues.forEach((issue) => console.log(issue))
      console.log()
    })

    console.log(`\n📊 Summary:`)
    console.log(`   Total files checked: ${files.length}`)
    console.log(`   Files with issues: ${results.length}`)
    console.log(`   Total issues found: ${totalIssues}`)

    if (FIX_MODE) {
      console.log(`   Files fixed: ${totalFixed}`)
    } else {
      console.log(`\n💡 Run with --fix to automatically fix these issues:`)
      console.log(`   node validate-icons.mjs --fix`)
    }
  }

  // Exit with error code if issues found and not in fix mode
  if (results.length > 0 && !FIX_MODE) {
    process.exit(1)
  }
}

main()
