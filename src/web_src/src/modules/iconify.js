import { addCollection } from '@iconify/vue'
import carbon from '@iconify-json/carbon/icons.json'
import iconParkOutline from '@iconify-json/icon-park-outline/icons.json'

// Register icon sets locally so @iconify/vue resolves them from the bundle
// instead of fetching from api.iconify.design at runtime (which fails in
// air-gapped/offline deployments). Keep this list in sync with the icon
// prefixes referenced across the app (menu/route icons, NovaIcon usages).
export function install() {
  addCollection(iconParkOutline)
  addCollection(carbon)
}
