import { REGION_MAP } from '@/lib/types';

export function getRegionVariants(region?: string) {
  if (!region || region === 'all') {
    return [];
  }

  const mappedRegion = REGION_MAP[region] ?? region;
  return mappedRegion === region ? [region] : [region, mappedRegion];
}

export function matchesRegion(candidateRegion: string, selectedRegion: string) {
  if (selectedRegion === 'all') {
    return true;
  }

  return getRegionVariants(selectedRegion).includes(candidateRegion);
}
