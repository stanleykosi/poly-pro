/**
 * @description
 * This client component encapsulates the TradingView Lightweight Charts library.
 * It provides a professional-grade charting solution with custom news event markers.
 *
 * Key features:
 * - Client-Side Rendering: Explicitly marked as a client component (`'use client'`)
 *   because the charting library directly interacts with the DOM.
 * - Historical Data Loading: Fetches historical OHLCV data from our backend API.
 * - Real-time Updates: Subscribes to WebSocket data via Zustand store for live price updates.
 * - News Event Markers: Displays news events as custom markers on the chart with tooltips.
 * - Dark Theme: Configured to match the application's dark theme design system.
 * - Lifecycle Management: Properly cleans up chart resources when component unmounts.
 *
 * @dependencies
 * - react: For `useEffect`, `useRef`, and component logic.
 * - lightweight-charts: The TradingView Lightweight Charts library.
 * - @/lib/lw-chart-datafeed: The datafeed adapter for fetching and updating data.
 * - @/types: Contains the `NewsEvent` type definition.
 *
 * @notes
 * - This implementation uses TradingView Lightweight Charts, which is free and open-source.
 * - News markers are implemented as custom markers using the library's marker API.
 * - The component handles data fetching, real-time updates, and marker rendering.
 */

'use client'

import React, { useEffect, useRef, useState } from 'react'
import {
  createChart,
  IChartApi,
  ISeriesApi,
  CandlestickData,
  Time,
  MarkerPosition,
  MarkerShape,
} from 'lightweight-charts'
import {
  fetchHistoricalData,
  subscribeToRealtimeUpdates,
  unsubscribeFromRealtimeUpdates,
  BarData,
} from '@/lib/lw-chart-datafeed'
import { NewsEvent } from '@/types'

interface LightweightChartProps {
  marketId: string
  newsEvents?: NewsEvent[]
}

const LightweightChart: React.FC<LightweightChartProps> = ({
  marketId,
  newsEvents = [],
}) => {
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<IChartApi | null>(null)
  const seriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null)
  const markersRef = useRef<any[]>([])
  const subscriptionIdRef = useRef<string | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Default resolution (15 minutes)
  const resolution = '15'

  useEffect(() => {
    if (!containerRef.current) return

    // Initialize the chart
    const chart = createChart(containerRef.current, {
      layout: {
        background: { color: '#0D1117' },
        textColor: '#C9D1D9',
      },
      grid: {
        vertLines: { color: '#161B22' },
        horzLines: { color: '#161B22' },
      },
      width: containerRef.current.clientWidth,
      height: 400,
      timeScale: {
        timeVisible: true,
        secondsVisible: false,
      },
      rightPriceScale: {
        borderColor: '#161B22',
      },
    })

    chartRef.current = chart

    // Create candlestick series
    const candlestickSeries = chart.addCandlestickSeries({
      upColor: '#3FB950',
      downColor: '#F85149',
      borderVisible: false,
      wickUpColor: '#3FB950',
      wickDownColor: '#F85149',
    })

    seriesRef.current = candlestickSeries

    // Load historical data
    const loadHistoricalData = async () => {
      try {
        setIsLoading(true)
        setError(null)

        // Calculate time range (last 30 days)
        const to = Math.floor(Date.now() / 1000) // Current time in seconds
        const from = to - 30 * 24 * 60 * 60 // 30 days ago

        const bars = await fetchHistoricalData(
          marketId,
          from,
          to,
          resolution
        )

        if (bars.length === 0) {
          setError('No historical data available')
          setIsLoading(false)
          return
        }

        // Set data to the series
        candlestickSeries.setData(bars as CandlestickData[])

        // Store the last bar for real-time updates
        const lastBar = bars[bars.length - 1]
        subscriptionIdRef.current = subscribeToRealtimeUpdates(
          marketId,
          candlestickSeries,
          lastBar
        )

        // Add news event markers
        addNewsMarkers(candlestickSeries, newsEvents)

        setIsLoading(false)
      } catch (err) {
        console.error('Failed to load historical data:', err)
        setError('Failed to load chart data')
        setIsLoading(false)
      }
    }

    loadHistoricalData()

    // Handle window resize
    const handleResize = () => {
      if (containerRef.current && chart) {
        chart.applyOptions({
          width: containerRef.current.clientWidth,
        })
      }
    }

    window.addEventListener('resize', handleResize)

    // Cleanup function
    return () => {
      window.removeEventListener('resize', handleResize)
      if (subscriptionIdRef.current) {
        unsubscribeFromRealtimeUpdates(marketId)
      }
      if (chart) {
        chart.remove()
      }
    }
  }, [marketId]) // Re-initialize if marketId changes

  // Update news markers when newsEvents prop changes
  useEffect(() => {
    if (seriesRef.current && newsEvents.length > 0) {
      // Clear existing markers
      markersRef.current = []
      // Add new markers
      addNewsMarkers(seriesRef.current, newsEvents)
    }
  }, [newsEvents])

  /**
   * @function addNewsMarkers
   * @description Adds news event markers to the chart series.
   * @param {ISeriesApi} series - The chart series to add markers to.
   * @param {NewsEvent[]} events - Array of news events to display.
   */
  const addNewsMarkers = (
    series: ISeriesApi<'Candlestick'>,
    events: NewsEvent[]
  ) => {
    if (!events || events.length === 0) return

    const markers = events.map((event) => {
      // Convert publishedAt ISO string to timestamp
      const eventTime = new Date(event.publishedAt).getTime() // Get milliseconds
      const time = (eventTime / 1000) as Time // Convert to seconds for Time type

      return {
        time: time,
        position: 'belowBar' as MarkerPosition,
        color: '#58A6FF',
        shape: 'circle' as MarkerShape,
        size: 1,
        text: event.title.substring(0, 50) + (event.title.length > 50 ? '...' : ''), // Truncate long titles
      }
    })

    // Add markers to the series
    series.setMarkers(markers)
    markersRef.current = markers
  }

  return (
    <div className="relative h-[400px] w-full">
      {isLoading && (
        <div className="absolute inset-0 flex items-center justify-center bg-background/80">
          <p className="text-secondary">Loading chart data...</p>
        </div>
      )}
      {error && (
        <div className="absolute inset-0 flex items-center justify-center bg-background/80">
          <p className="text-destructive">{error}</p>
        </div>
      )}
      <div ref={containerRef} className="h-full w-full" />
    </div>
  )
}

export default LightweightChart

